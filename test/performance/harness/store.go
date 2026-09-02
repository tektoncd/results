/*
Copyright 2026 The Tekton Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"encoding/json"
	"io"
	"slices"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	record "github.com/tektoncd/results/pkg/api/server/v1alpha2/record"
	"github.com/tektoncd/results/pkg/watcher/convert"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"github.com/tektoncd/results/test/performance/generator"
	"github.com/tektoncd/results/test/performance/metrics"
)

// storeOps are the per-RPC metric names the store driver records.
const (
	opCreateResult = "create_result"
	opCreateRecord = "create_record"
	opUpdateRecord = "update_record"
	opUpdateResult = "update_result"
	opCreateChild  = "create_child_record"
)

// sendLogEntry is the actual record of what the store driver wrote for one
// instance. It joins 1:1 with generator.DatasetDefinition on UID so callers can
// verify the store faithfully persisted the golden dataset.
type sendLogEntry struct {
	Index        int      `json:"index"`
	UID          string   `json:"uid"`
	ResultName   string   `json:"result_name"`
	RecordName   string   `json:"record_name"`
	ChildRecords []string `json:"child_records"`
	UpdateCount  int      `json:"update_count"`
	Codes        []string `json:"codes"`
}

// sendLog is a concurrency-safe collector of send-log entries.
type sendLog struct {
	mu      sync.Mutex
	entries []sendLogEntry
}

func (l *sendLog) add(e sendLogEntry) {
	l.mu.Lock()
	l.entries = append(l.entries, e)
	l.mu.Unlock()
}

// write emits the send log as indented JSON, ordered by instance index so the
// output is stable across runs regardless of worker scheduling.
func (l *sendLog) write(w io.Writer) error {
	l.mu.Lock()
	entries := make([]sendLogEntry, len(l.entries))
	copy(entries, l.entries)
	l.mu.Unlock()

	slices.SortFunc(entries, func(a, b sendLogEntry) int { return a.Index - b.Index })
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(entries)
}

// storeInstance replays the watcher write lifecycle for one instance over gRPC:
// ensure the parent Result, create the top-level Record in a pending state, apply
// 1-3 terminal-progression updates (etag-chained), update the Result summary, and
// create the child TaskRun records. Every RPC is timed into m by op name.
func storeInstance(ctx context.Context, gc pb.ResultsClient, inst *generator.Instance, updates int, m *metrics.MetricSet) sendLogEntry {
	entry := sendLogEntry{
		Index:        inst.Index,
		UID:          inst.UID,
		ResultName:   inst.ResultName,
		RecordName:   inst.RecordName,
		ChildRecords: inst.ChildRecords,
		UpdateCount:  updates,
	}
	record := func(err error) { entry.Codes = append(entry.Codes, metrics.Classify(err)) }

	// 1. Ensure the parent Result exists (tolerate reruns via AlreadyExists).
	err := observe(m, opCreateResult, func() error {
		_, e := gc.CreateResult(ctx, &pb.CreateResultRequest{
			Parent: inst.Namespace,
			Result: &pb.Result{
				Name:    inst.ResultName,
				Summary: recordSummary(inst, pb.RecordSummary_UNKNOWN),
			},
		})
		return e
	})
	if status.Code(err) == codes.AlreadyExists {
		err = nil
	}
	record(err)

	// 2. Create the top-level Record in a pending state, then apply the terminal
	//    progression as etag-chained updates.
	stages := stagedPipelineRuns(inst.PipelineRun, updates)
	pending, convErr := convert.ToProto(stages[0])
	if convErr != nil {
		record(convErr)
		return entry
	}
	var current *pb.Record
	err = observe(m, opCreateRecord, func() error {
		rec, e := gc.CreateRecord(ctx, &pb.CreateRecordRequest{
			Parent: inst.ResultName,
			Record: &pb.Record{Name: inst.RecordName, Data: pending},
		})
		current = rec
		return e
	})
	record(err)

	for _, stage := range stages[1:] {
		data, e := convert.ToProto(stage)
		if e != nil {
			record(e)
			continue
		}
		if current == nil {
			// The create failed; nothing to update against.
			record(status.Error(codes.FailedPrecondition, "no record to update"))
			continue
		}
		current.Data = data
		err = observe(m, opUpdateRecord, func() error {
			updated, e := gc.UpdateRecord(ctx, &pb.UpdateRecordRequest{Record: current, Etag: current.GetEtag()})
			if e == nil {
				current = updated
			}
			return e
		})
		record(err)
	}

	// 3. Update the Result summary to the terminal status.
	err = observe(m, opUpdateResult, func() error {
		_, e := gc.UpdateResult(ctx, &pb.UpdateResultRequest{
			Name: inst.ResultName,
			Result: &pb.Result{
				Name:    inst.ResultName,
				Summary: recordSummary(inst, summaryStatus(inst.Outcome)),
			},
		})
		return e
	})
	record(err)

	// 4. Create the child TaskRun records under the same Result.
	for _, tr := range inst.TaskRuns {
		data, e := convert.ToProto(tr)
		if e != nil {
			record(e)
			continue
		}
		name := childRecordName(inst.ResultName, tr)
		err = observe(m, opCreateChild, func() error {
			_, e := gc.CreateRecord(ctx, &pb.CreateRecordRequest{
				Parent: inst.ResultName,
				Record: &pb.Record{Name: name, Data: data},
			})
			return e
		})
		if status.Code(err) == codes.AlreadyExists {
			err = nil
		}
		record(err)
	}

	return entry
}

// childRecordName mirrors the generator's child record naming.
func childRecordName(resultName string, tr *tektonv1.TaskRun) string {
	return record.FormatName(resultName, string(tr.UID))
}

// recordSummary builds the RecordSummary for the top-level record.
func recordSummary(inst *generator.Instance, s pb.RecordSummary_Status) *pb.RecordSummary {
	summary := &pb.RecordSummary{
		Record: inst.RecordName,
		Type:   convert.TypeName(inst.PipelineRun),
		Status: s,
	}
	if t := inst.PipelineRun.Status.StartTime; t != nil {
		summary.StartTime = timestamppb.New(t.Time)
	}
	if t := inst.PipelineRun.Status.CompletionTime; t != nil {
		summary.EndTime = timestamppb.New(t.Time)
	}
	return summary
}

// summaryStatus maps a generated outcome to the API record-summary status enum.
func summaryStatus(o generator.Outcome) pb.RecordSummary_Status {
	switch o {
	case generator.OutcomeSucceeded:
		return pb.RecordSummary_SUCCESS
	case generator.OutcomeFailed:
		return pb.RecordSummary_FAILURE
	case generator.OutcomeCancelled:
		return pb.RecordSummary_CANCELLED
	default:
		return pb.RecordSummary_UNKNOWN
	}
}

// stagedPipelineRuns returns the lifecycle stages to persist: index 0 is the
// pending state, the final element is the terminal object, and any in between are
// running snapshots. There are updates+1 stages, i.e. one create plus `updates`
// UpdateRecord calls.
func stagedPipelineRuns(terminal *tektonv1.PipelineRun, updates int) []*tektonv1.PipelineRun {
	if updates < 1 {
		updates = 1
	}
	running := terminal.DeepCopy()
	running.Status.CompletionTime = nil
	running.Status.Conditions = duckv1.Conditions{{
		Type:    apis.ConditionSucceeded,
		Status:  corev1.ConditionUnknown,
		Reason:  "Running",
		Message: "Not all Tasks have completed executing",
	}}

	stages := make([]*tektonv1.PipelineRun, 0, updates+1)
	stages = append(stages, running)
	for i := 1; i < updates; i++ {
		stages = append(stages, running.DeepCopy())
	}
	stages = append(stages, terminal)
	return stages
}

// updateCountFor derives a deterministic number of record updates (1-3) for an
// instance. It does not affect golden answers (only the final state persists) but
// keeps the write volume reproducible across runs.
func updateCountFor(index int) int {
	return 1 + index%3
}
