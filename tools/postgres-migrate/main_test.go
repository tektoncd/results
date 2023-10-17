//go:build e2e_migrate
// +build e2e_migrate

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tektoncd/results/pkg/api/server/db"
	server "github.com/tektoncd/results/pkg/api/server/v1alpha2"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/record"
	"github.com/tektoncd/results/pkg/api/server/v1alpha2/result"
	"github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/anypb"
	"gorm.io/gorm"
)

var (
	parent = "a"
)

func TestMigrate(t *testing.T) {
	mysqldb, err := openMysql()
	if err != nil {
		log.Fatalf("failed to open the mysql db: %v", err)
	}

	if err := clearDB(mysqldb, &db.Result{}); err != nil {
		t.Fatalf("error clearing MySQL Results: %v", err)
	}

	postgres, err := openPostgres()
	if err != nil {
		log.Fatalf("failed to open the postgres db: %v", err)
	}
	if err := clearDB(postgres, &db.Result{}); err != nil {
		t.Fatalf("error clearing MySQL Results: %v", err)
	}

	// Prepopulate Results
	results := []*db.Result{
		{
			Parent: parent,
			Name:   "not-migrated",
			ID:     id(),
			Annotations: db.Annotations{
				"d": "e",
			},
			CreatedTime: time.Unix(1234567890, 0),
			UpdatedTime: time.Unix(1234567890, 0),
			Etag:        "f",
		},
		{
			Parent: parent,
			Name:   "partially-migrated",
			ID:     id(),
		},
		{
			Parent: parent,
			Name:   "fully-migrated",
			ID:     id(),
		},
	}
	for _, r := range results {
		if out := mysqldb.Create(r); out.Error != nil {
			t.Fatal(err)
		}
	}
	for _, r := range results[1:] {
		if out := postgres.Create(r); out.Error != nil {
			t.Fatal(out.Error)
		}
	}

	// Generate records for each result.
	// First dimension corresponds to a Record above.
	var inRecords []*Record
	for i, x := range [][]struct {
		name string
		data []byte
	}{
		{
			{"taskrun-not-migrated", protodata(t, taskrunpb)},
			{"pipelinerun-not-migrated", protodata(t, pipelinerunpb)},
		},
		{
			{"taskrun-not-migrated", protodata(t, taskrunpb)},
			{"pipelinerun-migrated", protodata(t, pipelinerunpb)},
		},
		{
			{"taskrun-migrated", protodata(t, taskrunpb)},
			{"pipelinerun-migrated", protodata(t, pipelinerunpb)},
		},
	} {
		for _, y := range x {
			inRecords = append(inRecords, &Record{
				Parent:     results[i].Parent,
				ResultName: results[i].Name,
				ResultID:   results[i].ID,
				Result: db.Result{
					Parent: results[i].Parent,
					Name:   results[i].Name,
					ID:     results[i].ID,
				},

				ID:          id(),
				Name:        y.name,
				CreatedTime: time.Unix(1000000000, 0).UTC(),
				UpdatedTime: time.Unix(1000000000, 0).UTC(),
				Etag:        "etag",
				Data:        y.data,
			})
		}
	}
	for _, r := range inRecords {
		if out := mysqldb.Create(r); out.Error != nil {
			t.Fatal(err)
		}
	}

	// Populate existing postgres records - adjusted for proto -> json type
	// conversion.
	var outRecords []*db.Record
	for _, r := range inRecords {
		out := &db.Record{
			Parent:     r.Parent,
			ResultName: r.ResultName,
			ResultID:   r.ResultID,
			Result:     r.Result,

			ID:          r.ID,
			Name:        r.Name,
			CreatedTime: r.CreatedTime,
			UpdatedTime: r.UpdatedTime,
			Etag:        r.Etag,
		}
		if strings.Contains(r.Name, "taskrun") {
			out.Type = "tekton.dev/v1beta1.TaskRun"
			out.Data = jsondata(t, taskrun)
		} else {
			out.Type = "tekton.dev/v1beta1.PipelineRun"
			out.Data = jsondata(t, pipelinerun)
		}
		outRecords = append(outRecords, out)
	}
	for _, r := range outRecords[3:] {
		t.Log("creating ", r.Parent, r.ResultName, r.Name, r.ID)

		if out := postgres.Create(r); out.Error != nil {
			t.Fatal(out.Error)
		}
	}

	t.Run("dryrun", func(t *testing.T) {
		outcomes, err := migrate(mysqldb, postgres, false)
		if err != nil {
			t.Fatal(err)
		}
		logOutcome(t, outcomes)

		want := outcomeLog{
			"a/results/fully-migrated":                                  OutcomeAlreadyExists,
			"a/results/fully-migrated/records/pipelinerun-migrated":     OutcomeAlreadyExists,
			"a/results/fully-migrated/records/taskrun-migrated":         OutcomeAlreadyExists,
			"a/results/not-migrated":                                    OutcomeDryRun,
			"a/results/not-migrated/records/pipelinerun-not-migrated":   OutcomeDryRun,
			"a/results/not-migrated/records/taskrun-not-migrated":       OutcomeDryRun,
			"a/results/partially-migrated":                              OutcomeAlreadyExists,
			"a/results/partially-migrated/records/pipelinerun-migrated": OutcomeAlreadyExists,
			"a/results/partially-migrated/records/taskrun-not-migrated": OutcomeDryRun,
		}

		if diff := cmp.Diff(want, outcomes); diff != "" {
			t.Error(diff)
		}
	})

	t.Run("migrate", func(t *testing.T) {
		outcomes, err := migrate(mysqldb, postgres, true)
		if err != nil {
			t.Fatal(err)
		}
		logOutcome(t, outcomes)

		want := outcomeLog{
			"a/results/fully-migrated":                                  OutcomeAlreadyExists,
			"a/results/fully-migrated/records/pipelinerun-migrated":     OutcomeAlreadyExists,
			"a/results/fully-migrated/records/taskrun-migrated":         OutcomeAlreadyExists,
			"a/results/not-migrated":                                    OutcomeSuccess,
			"a/results/not-migrated/records/pipelinerun-not-migrated":   OutcomeSuccess,
			"a/results/not-migrated/records/taskrun-not-migrated":       OutcomeSuccess,
			"a/results/partially-migrated":                              OutcomeAlreadyExists,
			"a/results/partially-migrated/records/pipelinerun-migrated": OutcomeAlreadyExists,
			"a/results/partially-migrated/records/taskrun-not-migrated": OutcomeSuccess,
		}

		if diff := cmp.Diff(want, outcomes); diff != "" {
			t.Error(diff)
		}
	})

	t.Run("migrate-again", func(t *testing.T) {
		outcomes, err := migrate(mysqldb, postgres, true)
		if err != nil {
			t.Fatal(err)
		}
		logOutcome(t, outcomes)

		want := outcomeLog{
			"a/results/fully-migrated":                                  OutcomeAlreadyExists,
			"a/results/fully-migrated/records/pipelinerun-migrated":     OutcomeAlreadyExists,
			"a/results/fully-migrated/records/taskrun-migrated":         OutcomeAlreadyExists,
			"a/results/not-migrated":                                    OutcomeAlreadyExists,
			"a/results/not-migrated/records/pipelinerun-not-migrated":   OutcomeAlreadyExists,
			"a/results/not-migrated/records/taskrun-not-migrated":       OutcomeAlreadyExists,
			"a/results/partially-migrated":                              OutcomeAlreadyExists,
			"a/results/partially-migrated/records/pipelinerun-migrated": OutcomeAlreadyExists,
			"a/results/partially-migrated/records/taskrun-not-migrated": OutcomeAlreadyExists,
		}

		if diff := cmp.Diff(want, outcomes); diff != "" {
			t.Error(diff)
		}
	})

	t.Run("api", func(t *testing.T) {
		// Access new db via the API to make sure everything functions as expected.
		api, err := server.New(postgres)
		if err != nil {
			t.Fatalf("failed to create server: %v", err)
		}
		ctx := context.Background()

		// Results
		{
			resp, err := api.ListResults(ctx, &results_go_proto.ListResultsRequest{Parent: results[0].Parent})
			if err != nil {
				t.Fatalf("error listing API results: %v", err)
			}
			got := resp.Results
			sort.Slice(got, func(i, j int) bool {
				return got[i].GetId() < got[j].GetId()
			})
			want := make([]*pb.Result, 0, len(results))
			for _, r := range results {
				want = append(want, result.ToAPI(r))
			}
			if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
				t.Errorf("Results: %s", diff)
			}
		}
		// Records
		{
			resp, err := api.ListRecords(ctx, &results_go_proto.ListRecordsRequest{Parent: fmt.Sprintf("%s/results/-", results[0].Parent)})
			if err != nil {
				t.Fatalf("error listing API records: %v", err)
			}

			got := resp.Records
			// Stablize output so it matches the ordering we created above.
			sort.Slice(got, func(i, j int) bool {
				return got[i].GetId() < got[j].GetId()
			})

			want := make([]*pb.Record, 0, len(outRecords))
			for _, r := range outRecords {
				api, err := record.ToAPI(r)
				if err != nil {
					t.Fatalf("error converting Records to API type: %v", err)
				}
				want = append(want, api)
			}
			// Ignore the data value for now - we can't guarantee the field
			// ordering so we need to inspect this separately.
			if diff := cmp.Diff(want, got, protocmp.Transform(), protocmp.IgnoreFields(&pb.Any{}, "value")); diff != "" {
				t.Errorf("Records: %s", diff)
			}
			// Verify data values.
			for _, r := range got {
				t.Run(r.GetName(), func(t *testing.T) {
					if strings.Contains(r.Name, "taskrun") {
						tr := new(v1beta1.TaskRun)
						if err := json.Unmarshal(r.Data.Value, tr); err != nil {
							t.Fatalf("error unmarshalling JSON: %v", err)
						}
						if diff := cmp.Diff(taskrun, tr); diff != "" {
							t.Error(diff)
						}
					} else {
						pr := new(v1beta1.PipelineRun)
						if err := json.Unmarshal(r.Data.Value, pr); err != nil {
							t.Fatalf("error unmarshalling JSON: %v", err)
						}
						if diff := cmp.Diff(pipelinerun, pr); diff != "" {
							t.Error(diff)
						}
					}
				})
			}
		}
	})
}

func clearDB(db *gorm.DB, types ...any) error {
	for _, t := range types {
		out := db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(t)
		if out.Error != nil {
			return out.Error
		}
	}
	return nil
}

var idx uint32

func id() string {
	return fmt.Sprint(atomic.AddUint32(&idx, 1))
}

func protodata(t *testing.T, m proto.Message) []byte {
	a, err := anypb.New(m)
	if err != nil {
		t.Fatalf("error creating anypb: %v", err)
	}
	b, err := proto.Marshal(a)
	if err != nil {
		t.Fatalf("error marshalling anypb: %v", err)
	}
	return b
}

func jsondata(t *testing.T, v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("error marshinalling json: %v", err)
	}
	return b
}

func logOutcome(t *testing.T, outcomes outcomeLog) {
	if b, err := json.MarshalIndent(outcomes, "", "  "); err == nil {
		t.Logf("Outcome:\n%s", string(b))
	} else {
		t.Log(outcomes)
	}
}
