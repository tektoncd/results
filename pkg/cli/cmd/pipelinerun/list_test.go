package pipelinerun

import (
	"strings"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/spf13/cobra"
	"github.com/tektoncd/results/pkg/cli/testutils"
	"github.com/tektoncd/results/pkg/test"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
)

func TestListPipelineRunsCommand(t *testing.T) {
	// Simple test to verify command structure and basic functionality
	params := testutils.NewParams()

	// Create command
	cmd := Command(params)

	// Verify command is created properly
	if cmd == nil {
		t.Fatal("Command should not be nil")
	}

	if cmd.Use != "pipelinerun" {
		t.Errorf("Expected command use to be 'pipelinerun', got %s", cmd.Use)
	}

	// Check that subcommands are added
	subcommands := cmd.Commands()
	if len(subcommands) == 0 {
		t.Error("Expected subcommands to be added")
	}

	// Find the list subcommand
	var listCmd *cobra.Command
	for _, subcmd := range subcommands {
		if subcmd.Use == "list [pipeline-name]" {
			listCmd = subcmd
			break
		}
	}

	if listCmd == nil {
		t.Fatal("Expected 'list' subcommand to be present")
	}

	// Verify list command has the right aliases
	if len(listCmd.Aliases) == 0 || listCmd.Aliases[0] != "ls" {
		t.Error("Expected 'list' command to have 'ls' alias")
	}
}

// TestListPipelineRunsScenarios covers various scenarios and use cases
func TestListPipelineRunsScenarios(t *testing.T) {
	clock := clockwork.NewFakeClock()

	type testCase struct {
		name           string
		records        []*pb.Record
		args           []string
		expectedOutput string
		expectError    bool
		errorMessage   string
	}

	tests := []testCase{
		{
			name: "multiple_pipelineruns_different_statuses",
			records: []*pb.Record{
				testutils.CreateTestRecord(clock, "successful-run", "uid-1", "",
					testutils.TimePtr(clock.Now().Add(-10*time.Minute)), testutils.TimePtr(clock.Now().Add(-8*time.Minute)), "True", nil),
				testutils.CreateTestRecord(clock, "failed-run", "uid-2", "",
					testutils.TimePtr(clock.Now().Add(-6*time.Minute)), testutils.TimePtr(clock.Now().Add(-4*time.Minute)), "False", nil),
				testutils.CreateTestRecord(clock, "running-run", "uid-3", "",
					testutils.TimePtr(clock.Now().Add(-3*time.Minute)), nil, "Unknown", nil), // Running pipeline
			},
			args: []string{"list"},
			expectedOutput: `NAME             UID     STARTED   DURATION   STATUS
successful-run   uid-1   10m ago   2m0s       Succeeded
failed-run       uid-2   6m ago    2m0s       Failed
running-run      uid-3   3m ago    ---        Running
`,
		},
		{
			name:    "empty_list_no_pipelineruns",
			records: []*pb.Record{},
			args:    []string{"list"},
			expectedOutput: `No PipelineRuns found
`,
		},
		{
			name: "list_with_alias_ls",
			records: []*pb.Record{
				testutils.CreateTestRecord(clock, "test-run", "uid-4", "",
					testutils.TimePtr(clock.Now().Add(-5*time.Minute)), testutils.TimePtr(clock.Now().Add(-3*time.Minute)), "True", nil),
			},
			args: []string{"ls"},
			expectedOutput: `NAME       UID     STARTED   DURATION   STATUS
test-run   uid-4   5m ago    2m0s       Succeeded
`,
		},
		{
			name: "pipeline_with_no_start_time",
			records: []*pb.Record{
				testutils.CreateTestRecord(clock, "no-start-time-run", "uid-5", "", nil, nil, "True", nil),
			},
			args: []string{"list"},
			expectedOutput: `NAME                UID     STARTED   DURATION   STATUS
no-start-time-run   uid-5             ---        Succeeded
`,
		},
		{
			name: "pipeline_with_zero_duration",
			records: []*pb.Record{
				testutils.CreateTestRecord(clock, "instant-run", "uid-6", "",
					testutils.TimePtr(clock.Now().Add(-1*time.Minute)), testutils.TimePtr(clock.Now().Add(-1*time.Minute)), "True", nil), // Same start and end time
			},
			args: []string{"list"},
			expectedOutput: `NAME          UID     STARTED   DURATION   STATUS
instant-run   uid-6   1m ago    0s         Succeeded
`,
		},
		{
			name: "filter_by_pipeline_name",
			records: []*pb.Record{
				testutils.CreateTestRecord(clock, "build-pipeline-1", "uid-7", "",
					testutils.TimePtr(clock.Now().Add(-5*time.Minute)), testutils.TimePtr(clock.Now().Add(-3*time.Minute)), "True", nil),
				testutils.CreateTestRecord(clock, "test-pipeline-1", "uid-8", "",
					testutils.TimePtr(clock.Now().Add(-4*time.Minute)), testutils.TimePtr(clock.Now().Add(-2*time.Minute)), "True", nil),
			},
			args: []string{"list", "build-pipeline"},
			expectedOutput: `NAME               UID     STARTED   DURATION   STATUS
build-pipeline-1   uid-7   5m ago    2m0s       Succeeded
`,
		},
		{
			name: "filter_by_pipeline_name_no_matches",
			records: []*pb.Record{
				testutils.CreateTestRecord(clock, "build-pipeline-1", "uid-7a", "",
					testutils.TimePtr(clock.Now().Add(-5*time.Minute)), testutils.TimePtr(clock.Now().Add(-3*time.Minute)), "True", nil),
				testutils.CreateTestRecord(clock, "test-pipeline-1", "uid-8a", "",
					testutils.TimePtr(clock.Now().Add(-4*time.Minute)), testutils.TimePtr(clock.Now().Add(-2*time.Minute)), "True", nil),
			},
			args: []string{"list", "deploy-pipeline"},
			// No pipelines match "deploy-pipeline", so should show empty result
			expectedOutput: `No PipelineRuns found
`,
		},
		{
			name: "filter_by_pipeline_name_partial_match",
			records: []*pb.Record{
				testutils.CreateTestRecord(clock, "my-build-pipeline", "uid-9a", "",
					testutils.TimePtr(clock.Now().Add(-6*time.Minute)), testutils.TimePtr(clock.Now().Add(-4*time.Minute)), "True", nil),
				testutils.CreateTestRecord(clock, "build-and-deploy", "uid-9b", "",
					testutils.TimePtr(clock.Now().Add(-5*time.Minute)), testutils.TimePtr(clock.Now().Add(-3*time.Minute)), "True", nil),
				testutils.CreateTestRecord(clock, "test-suite", "uid-9c", "",
					testutils.TimePtr(clock.Now().Add(-4*time.Minute)), testutils.TimePtr(clock.Now().Add(-2*time.Minute)), "True", nil),
			},
			args: []string{"list", "build"},
			// Should match both "my-build-pipeline" and "build-and-deploy" (contains "build")
			expectedOutput: `NAME                UID      STARTED   DURATION   STATUS
my-build-pipeline   uid-9a   6m ago    2m0s       Succeeded
build-and-deploy    uid-9b   5m ago    2m0s       Succeeded
`,
		},
		{
			name:         "invalid_limit_too_low",
			records:      []*pb.Record{},
			args:         []string{"list", "--limit=3"},
			expectError:  true,
			errorMessage: "limit should be between 5 and 1000",
		},
		{
			name:         "invalid_limit_too_high",
			records:      []*pb.Record{},
			args:         []string{"list", "--limit=1001"},
			expectError:  true,
			errorMessage: "limit should be between 5 and 1000",
		},
		{
			name: "namespace_filtering",
			records: []*pb.Record{
				testutils.CreateTestRecord(clock, "prod-run", "uid-9", "production",
					testutils.TimePtr(clock.Now().Add(-3*time.Minute)), testutils.TimePtr(clock.Now().Add(-1*time.Minute)), "True", nil),
				testutils.CreateTestRecord(clock, "dev-run", "uid-10", "development",
					testutils.TimePtr(clock.Now().Add(-2*time.Minute)), testutils.TimePtr(clock.Now().Add(-1*time.Minute)), "True", nil),
			},
			args: []string{"list", "-n", "production"},
			expectedOutput: `NAME       UID     STARTED   DURATION   STATUS
prod-run   uid-9   3m ago    2m0s       Succeeded
`,
		},
		{
			name: "all_namespaces_shows_both",
			records: []*pb.Record{
				testutils.CreateTestRecord(clock, "prod-run", "uid-11", "production",
					testutils.TimePtr(clock.Now().Add(-3*time.Minute)), testutils.TimePtr(clock.Now().Add(-1*time.Minute)), "True", nil),
				testutils.CreateTestRecord(clock, "dev-run", "uid-12", "development",
					testutils.TimePtr(clock.Now().Add(-2*time.Minute)), testutils.TimePtr(clock.Now().Add(-1*time.Minute)), "True", nil),
			},
			args: []string{"list", "-A"},
			expectedOutput: `NAMESPACE     NAME       UID      STARTED   DURATION   STATUS
production    prod-run   uid-11   3m ago    2m0s       Succeeded
development   dev-run    uid-12   2m ago    1m0s       Succeeded
`,
		},
		{
			name: "namespace_filtering_empty_result",
			records: []*pb.Record{
				testutils.CreateTestRecord(clock, "prod-run", "uid-13", "production",
					testutils.TimePtr(clock.Now().Add(-3*time.Minute)), testutils.TimePtr(clock.Now().Add(-1*time.Minute)), "True", nil),
			},
			args: []string{"list", "-n", "nonexistent-namespace"},
			expectedOutput: `No PipelineRuns found
`,
		},
		{
			name:         "conflicting_namespace_flags",
			records:      []*pb.Record{},
			args:         []string{"list", "-A", "-n", "test-namespace"},
			expectError:  true,
			errorMessage: "cannot use --all-namespaces/-A and --namespace/-n together",
		},
		{
			name: "label_filtering",
			records: []*pb.Record{
				testutils.CreateTestRecord(clock, "labeled-run", "uid-14", "",
					testutils.TimePtr(clock.Now().Add(-4*time.Minute)), testutils.TimePtr(clock.Now().Add(-2*time.Minute)), "True",
					map[string]string{"app": "myapp", "env": "prod"}),
				testutils.CreateTestRecord(clock, "other-run", "uid-15", "",
					testutils.TimePtr(clock.Now().Add(-3*time.Minute)), testutils.TimePtr(clock.Now().Add(-1*time.Minute)), "True",
					map[string]string{"app": "other", "env": "dev"}),
			},
			args: []string{"list", "-L", "app=myapp"},
			expectedOutput: `NAME          UID      STARTED   DURATION   STATUS
labeled-run   uid-14   4m ago    2m0s       Succeeded
`,
		},
		{
			name: "label_filtering_multiple_labels",
			records: []*pb.Record{
				testutils.CreateTestRecord(clock, "prod-app", "uid-16", "",
					testutils.TimePtr(clock.Now().Add(-5*time.Minute)), testutils.TimePtr(clock.Now().Add(-3*time.Minute)), "True",
					map[string]string{"app": "myapp", "env": "prod"}),
				testutils.CreateTestRecord(clock, "dev-app", "uid-17", "",
					testutils.TimePtr(clock.Now().Add(-4*time.Minute)), testutils.TimePtr(clock.Now().Add(-2*time.Minute)), "True",
					map[string]string{"app": "myapp", "env": "dev"}),
				testutils.CreateTestRecord(clock, "other-prod", "uid-18", "",
					testutils.TimePtr(clock.Now().Add(-3*time.Minute)), testutils.TimePtr(clock.Now().Add(-1*time.Minute)), "True",
					map[string]string{"app": "other", "env": "prod"}),
			},
			args: []string{"list", "-L", "app=myapp,env=prod"},
			// Should match only the record with both app=myapp AND env=prod
			expectedOutput: `NAME       UID      STARTED   DURATION   STATUS
prod-app   uid-16   5m ago    2m0s       Succeeded
`,
		},
		{
			name: "label_filtering_no_matches",
			records: []*pb.Record{
				testutils.CreateTestRecord(clock, "labeled-run", "uid-19", "",
					testutils.TimePtr(clock.Now().Add(-4*time.Minute)), testutils.TimePtr(clock.Now().Add(-2*time.Minute)), "True",
					map[string]string{"app": "myapp", "env": "prod"}),
			},
			args: []string{"list", "-L", "app=nonexistent"},
			// No records match the label filter
			expectedOutput: `No PipelineRuns found
`,
		},
		{
			name: "label_filtering_no_labels_on_record",
			records: []*pb.Record{
				testutils.CreateTestRecord(clock, "unlabeled-run", "uid-20", "",
					testutils.TimePtr(clock.Now().Add(-4*time.Minute)), testutils.TimePtr(clock.Now().Add(-2*time.Minute)), "True", nil),
			},
			args: []string{"list", "-L", "app=myapp"},
			// Record has no labels, so should not match
			expectedOutput: `No PipelineRuns found
`,
		},
		{
			name:         "invalid_label_format",
			records:      []*pb.Record{},
			args:         []string{"list", "-L", "invalid-label-format"},
			expectError:  true,
			errorMessage: "invalid label format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test params with mock REST client
			params := testutils.NewParams()

			mockRESTClient, err := testutils.MockRESTClientFromRecords(tt.records)
			if err != nil {
				t.Fatalf("Failed to create mock REST client: %v", err)
			}
			params.SetRESTClient(mockRESTClient)

			// Execute the list command
			cmd := Command(params)

			output, err := testutils.ExecuteCommand(cmd, tt.args...)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errorMessage != "" && !strings.Contains(err.Error(), tt.errorMessage) {
					t.Errorf("Expected error message to contain %q, got %q", tt.errorMessage, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			test.AssertOutput(t, tt.expectedOutput, output)
		})
	}
}
