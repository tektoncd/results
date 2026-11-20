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
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/tektoncd/results/test/performance/benchmark/workload"
)

// newDeriveCmd builds `bench derive`, which turns raw KubeArchive access logs into
// an anonymized query-mix spec (see the workload package). It needs no cluster.
func newDeriveCmd() *cobra.Command {
	var input, output string
	cmd := &cobra.Command{
		Use:   "derive",
		Short: "Derive an anonymized query-mix spec from KubeArchive access logs",
		Long: "Derive scans KubeArchive access-log lines and emits an anonymized " +
			"QueryMixSpec (kinds, label keys, page sizes, and namespace fan-out as " +
			"counts only). Raw namespaces, label values, tokens, and UIDs are never " +
			"written to the spec.",
		RunE: func(_ *cobra.Command, _ []string) error {
			in := os.Stdin
			if input != "" {
				f, err := os.Open(input) //nolint:gosec // operator-provided log path
				if err != nil {
					return err
				}
				defer func() { _ = f.Close() }()
				in = f
			}
			spec, err := workload.Derive(in)
			if err != nil {
				return err
			}
			if input != "" {
				spec.Source = fmt.Sprintf("derived from %s", filepath.Base(input))
			}
			return withOutput(output, func(w *os.File) error {
				return workload.Write(w, spec)
			})
		},
	}
	cmd.Flags().StringVar(&input, "input", "", "access-log file to read; empty reads stdin")
	cmd.Flags().StringVar(&output, "output", "", "spec output path; empty writes /tmp/<timestamp>-<commit>.json")
	return cmd
}

// newCompareCmd builds `bench compare`, the milestone-gate regression checker. It
// exits non-zero when any metric regresses beyond the threshold so CI can gate on
// it. It needs no cluster.
func newCompareCmd() *cobra.Command {
	threshold := defaultThreshold
	cmd := &cobra.Command{
		Use:   "compare <baseline.json> <candidate.json>",
		Short: "Compare two benchmark reports and flag regressions",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			baseline, err := readReport(args[0])
			if err != nil {
				return err
			}
			candidate, err := readReport(args[1])
			if err != nil {
				return err
			}
			c := compareReports(baseline, candidate, threshold)
			writeComparison(cmd.OutOrStdout(), c)
			if regressed := c.regressions(); len(regressed) > 0 {
				return fmt.Errorf("%d metric(s) regressed beyond %.0f%% threshold", len(regressed), threshold*100)
			}
			return nil
		},
	}
	cmd.Flags().Float64Var(&threshold, "threshold", defaultThreshold, "relative change flagged as a regression (0.10 = ±10%)")
	return cmd
}
