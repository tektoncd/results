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

// Command bench is the Tekton Results performance benchmark harness. It drives
// the real deployed API surface (gRPC for store, REST/gRPC for query) against a
// deterministic, versioned dataset and emits machine-readable JSON reports.
//
// Subcommands:
//
//	bench load  --verify   load the seed dataset through the API and verify counts
//	bench store            replay the watcher write lifecycle over the live range
//	bench query            run the predefined read mix against the seed range
//	bench mixed            run writers (live) and readers (seed) in parallel
//	bench dataset          emit the golden DatasetDefinition (no cluster needed)
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/tektoncd/results/test/performance/generator"
	"github.com/tektoncd/results/test/performance/metrics"
	"github.com/tektoncd/results/test/performance/report"
)

const defaultReadDuration = 30 * time.Second

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return newRootCmd().ExecuteContext(ctx)
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "bench",
		Short:         "Tekton Results performance benchmark harness",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newLoadCmd(), newStoreCmd(), newQueryCmd(), newMixedCmd(), newDatasetCmd())
	return root
}

func newStoreCmd() *cobra.Command {
	cfg := &RunConfig{}
	cmd := &cobra.Command{
		Use:   "store",
		Short: "Benchmark write/ingest by replaying the watcher lifecycle over gRPC",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			clients, err := NewAPIClients(cfg.clientConfig())
			if err != nil {
				return err
			}
			g, err := newGenerator(cfg, true) // store writes the live range
			if err != nil {
				return err
			}
			log := &sendLog{}
			m := runIndexed(ctx, cfg.Concurrency, 0, cfg.Count, func(ctx context.Context, index int, m *metrics.MetricSet) {
				inst, err := g.At(index)
				if err != nil {
					m.Observe("generate", 0, err)
					return
				}
				log.add(storeInstance(ctx, clients.GRPC, inst, updateCountFor(index), m))
			})
			if err := writeSendLog(cfg.OutputPath, log); err != nil {
				return err
			}
			return emitReport(ctx, cfg, "store", "grpc", reportExtra{}, m.Snapshot())
		},
	}
	bindCommon(cmd, cfg)
	return cmd
}

func newQueryCmd() *cobra.Command {
	cfg := &RunConfig{}
	var (
		transport string
		pageSize  int32
	)
	cmd := &cobra.Command{
		Use:   "query",
		Short: "Benchmark read/list performance against the seed range",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			clients, err := NewAPIClients(cfg.clientConfig())
			if err != nil {
				return err
			}
			pick, err := listerPicker(transport, clients)
			if err != nil {
				return err
			}
			g, err := newGenerator(cfg, false)
			if err != nil {
				return err
			}
			namespaces := g.Config().Namespaces
			queries := defaultQueries()

			m := runTimed(ctx, cfg.Concurrency, readDuration(cfg), func(ctx context.Context, workerID, iter int, m *metrics.MetricSet) {
				q := queries[iter%len(queries)]
				ns := namespaces[(workerID+iter)%len(namespaces)]
				runQuery(ctx, pick(iter), q, ns, pageSize, m)
			})
			return emitReport(ctx, cfg, "query", transport, reportExtra{}, m.Snapshot())
		},
	}
	bindCommon(cmd, cfg)
	cmd.Flags().StringVar(&transport, "transport", "grpc", "query transport: grpc|rest|both")
	cmd.Flags().Int32Var(&pageSize, "page-size", 1000, "list page size")
	return cmd
}

func newMixedCmd() *cobra.Command {
	cfg := &RunConfig{}
	var (
		readRatio  int
		writeRatio int
		pageSize   int32
	)
	cmd := &cobra.Command{
		Use:   "mixed",
		Short: "Benchmark parallel store (live range) and query (seed range)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			clients, err := NewAPIClients(cfg.clientConfig())
			if err != nil {
				return err
			}
			live, err := newGenerator(cfg, true)
			if err != nil {
				return err
			}
			seed, err := newGenerator(cfg, false)
			if err != nil {
				return err
			}
			m := runMixed(ctx, clients.GRPC, live, seed, mixedParams{
				Concurrency: cfg.Concurrency,
				Duration:    readDuration(cfg),
				ReadRatio:   readRatio,
				WriteRatio:  writeRatio,
				PageSize:    pageSize,
			})
			return emitReport(ctx, cfg, "mixed", "grpc", reportExtra{readRatio: readRatio, writeRatio: writeRatio}, m.Snapshot())
		},
	}
	bindCommon(cmd, cfg)
	cmd.Flags().IntVar(&readRatio, "read-ratio", 3, "reader share of the worker budget")
	cmd.Flags().IntVar(&writeRatio, "write-ratio", 1, "writer share of the worker budget")
	cmd.Flags().Int32Var(&pageSize, "page-size", 1000, "list page size")
	return cmd
}

func newLoadCmd() *cobra.Command {
	cfg := &RunConfig{}
	var verify bool
	cmd := &cobra.Command{
		Use:   "load",
		Short: "Load the seed dataset through the API and optionally verify row counts",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			clients, err := NewAPIClients(cfg.clientConfig())
			if err != nil {
				return err
			}
			m, def, err := loadSeed(ctx, clients.GRPC, cfg)
			if err != nil {
				return err
			}
			if verify {
				if err := verifySeed(ctx, grpcLister{c: clients.GRPC}, def); err != nil {
					return fmt.Errorf("seed verification failed: %w", err)
				}
				fmt.Fprintf(os.Stderr, "seed verified: %d PipelineRuns, dataset hash %s\n", def.Count, def.ContentHash)
			}
			return emitReport(ctx, cfg, "load", "grpc", reportExtra{datasetHash: def.ContentHash}, m.Snapshot())
		},
	}
	bindCommon(cmd, cfg)
	cmd.Flags().BoolVar(&verify, "verify", false, "verify record counts against the golden definition after loading")
	return cmd
}

func newDatasetCmd() *cobra.Command {
	cfg := &RunConfig{}
	cmd := &cobra.Command{
		Use:   "dataset",
		Short: "Emit the golden DatasetDefinition (no cluster required)",
		RunE: func(_ *cobra.Command, _ []string) error {
			g, err := newGenerator(cfg, false)
			if err != nil {
				return err
			}
			def, err := g.Definition()
			if err != nil {
				return err
			}
			return withOutput(cfg.OutputPath, func(w *os.File) error {
				return generator.WriteDefinition(w, def)
			})
		},
	}
	bindCommon(cmd, cfg)
	return cmd
}

// reportExtra carries mode-specific report fields.
type reportExtra struct {
	datasetHash string
	readRatio   int
	writeRatio  int
}

// emitReport assembles and writes the JSON report for a completed run.
func emitReport(ctx context.Context, cfg *RunConfig, mode, transport string, extra reportExtra, snap metrics.Snapshot) error {
	meta := report.CaptureMeta(ctx)
	meta.DatasetVersion = cfg.DatasetVersion
	meta.DatasetHash = extra.datasetHash
	meta.Tier = cfg.Tier
	meta.Mode = mode
	meta.APIServerAddr = cfg.ServerAddress
	meta.DBBackend = cfg.DBBackend
	meta.ServerImage = cfg.ServerImage

	rc := report.Config{
		Count:       cfg.Count,
		Concurrency: cfg.Concurrency,
		DurationSec: cfg.DurationSec,
		Transport:   transport,
		ReadRatio:   extra.readRatio,
		WriteRatio:  extra.writeRatio,
		Dataset:     cfg.Dataset,
		Seed:        cfg.Seed,
	}
	rep := report.New(meta, rc, snap)
	return withOutput(cfg.OutputPath, func(w *os.File) error {
		return report.Write(w, rep)
	})
}

// listerPicker returns a function selecting the transport for a given iteration.
func listerPicker(transport string, clients *APIClients) (func(iter int) lister, error) {
	g := grpcLister{c: clients.GRPC}
	switch transport {
	case "grpc":
		return func(int) lister { return g }, nil
	case "rest":
		return func(int) lister { return clients.REST }, nil
	case "both":
		return func(iter int) lister {
			if iter%2 == 0 {
				return g
			}
			return clients.REST
		}, nil
	default:
		return nil, fmt.Errorf("unknown transport %q (want grpc|rest|both)", transport)
	}
}

// readDuration returns the configured duration, defaulting when unset.
func readDuration(cfg *RunConfig) time.Duration {
	if cfg.DurationSec <= 0 {
		return defaultReadDuration
	}
	return time.Duration(cfg.DurationSec) * time.Second
}

// writeSendLog writes the store driver's actual send log alongside the report so
// it can be joined against the golden DatasetDefinition on UID. It is skipped when
// the report goes to stdout (no path to derive a sidecar from).
func writeSendLog(reportPath string, log *sendLog) error {
	if reportPath == "" {
		return nil
	}
	path := strings.TrimSuffix(reportPath, filepath.Ext(reportPath)) + ".sendlog.json"
	return withOutput(path, func(w *os.File) error { return log.write(w) })
}

// withOutput invokes fn with the report sink: a file when path is set, else stdout.
func withOutput(path string, fn func(*os.File) error) error {
	if path == "" {
		return fn(os.Stdout)
	}
	f, err := os.Create(path) //nolint:gosec // operator-provided report output path
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return fn(f)
}
