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
	"os"

	"github.com/spf13/cobra"
)

// RunConfig holds the flags shared by every benchmark subcommand. Endpoint and
// auth flags default from the same environment variables the e2e suite uses so a
// harness run drops into an existing kind setup with no extra configuration.
type RunConfig struct {
	// Connection / auth.
	ServerAddress string
	ServerName    string
	CertFile      string
	TokenFile     string

	// Workload shape.
	Concurrency int
	Count       int
	DurationSec int

	// Dataset (shared by generator, loader, and query golden answers).
	Seed           int64
	DatasetVersion string
	Dataset        string
	Namespaces     int
	ChildMin       int
	ChildMax       int

	// Reporting.
	Tier        string
	OutputPath  string
	DBBackend   string
	ServerImage string
}

// bindCommon registers the shared flags on cmd and wires their defaults from the
// environment. Subcommands add their own flags (transport, ratios) on top.
func bindCommon(cmd *cobra.Command, cfg *RunConfig) {
	f := cmd.Flags()
	f.StringVar(&cfg.ServerAddress, "server-addr", envOr("API_SERVER_ADDR", "https://localhost:8080"), "API server address (env API_SERVER_ADDR)")
	f.StringVar(&cfg.ServerName, "server-name", envOr("API_SERVER_NAME", "tekton-results-api-service.tekton-pipelines.svc.cluster.local"), "TLS server name (env API_SERVER_NAME)")
	f.StringVar(&cfg.CertFile, "cert", os.Getenv("SSL_CERT_PATH"), "CA certificate file; empty skips verification (env SSL_CERT_PATH)")
	f.StringVar(&cfg.TokenFile, "token", os.Getenv("SA_TOKEN_PATH"), "bearer token file (env SA_TOKEN_PATH)")

	f.IntVar(&cfg.Concurrency, "concurrency", 8, "number of concurrent workers")
	f.IntVar(&cfg.Count, "count", 1000, "number of top-level PipelineRuns for count-bounded runs")
	f.IntVar(&cfg.DurationSec, "duration", 0, "run duration in seconds for time-bounded runs (0 = count-bounded)")

	f.Int64Var(&cfg.Seed, "seed", 42, "deterministic generator seed")
	f.StringVar(&cfg.DatasetVersion, "dataset-version", "seed-small-v1", "dataset content version tag")
	f.StringVar(&cfg.Dataset, "dataset", "seed-small", "named dataset tier")
	f.IntVar(&cfg.Namespaces, "namespaces", 50, "number of namespaces to spread runs across")
	f.IntVar(&cfg.ChildMin, "child-min", 2, "minimum child TaskRuns per PipelineRun")
	f.IntVar(&cfg.ChildMax, "child-max", 15, "maximum child TaskRuns per PipelineRun")

	f.StringVar(&cfg.Tier, "tier", "tier1", "environment tier label for the report")
	f.StringVar(&cfg.OutputPath, "output", "", "report output path; empty writes to stdout")
	f.StringVar(&cfg.DBBackend, "db-backend", envOr("BENCH_DB_BACKEND", "local"), "database backend label: local|external (env BENCH_DB_BACKEND)")
	f.StringVar(&cfg.ServerImage, "server-image", os.Getenv("BENCH_SERVER_IMAGE"), "server image tag for the report (env BENCH_SERVER_IMAGE)")
}

// clientConfig extracts the connection settings.
func (c *RunConfig) clientConfig() ClientConfig {
	return ClientConfig{
		ServerAddress: c.ServerAddress,
		ServerName:    c.ServerName,
		CertFile:      c.CertFile,
		TokenFile:     c.TokenFile,
	}
}

// envOr returns the environment variable value or a fallback.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
