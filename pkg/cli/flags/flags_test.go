package flags

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/tektoncd/results/pkg/cli/common"
)

// TestAddResultsOptions tests that the AddResultsOptions function correctly adds flags to a command
func TestAddResultsOptions(t *testing.T) {
	tests := []struct {
		name          string
		cmd           *cobra.Command
		wantFlags     []string
		wantShorthand map[string]string
	}{
		{
			name: "add all flags",
			cmd: &cobra.Command{
				Use:   "test",
				Short: "Test command",
			},
			wantFlags: []string{
				kubeConfig,
				context,
				namespace,
				host,
				token,
				apiPath,
				insecureSkipTLSVerify,
			},
			wantShorthand: map[string]string{
				kubeConfig: "k",
				context:    "c",
				namespace:  "n",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Add the flags
			AddResultsOptions(tt.cmd)

			// Check that all expected flags are added
			for _, flagName := range tt.wantFlags {
				flag := tt.cmd.PersistentFlags().Lookup(flagName)
				if flag == nil {
					t.Errorf("Expected flag %q to be added, but it was not", flagName)
				}
			}

			// Check flag shorthand
			for flagName, wantShorthand := range tt.wantShorthand {
				flag := tt.cmd.PersistentFlags().Lookup(flagName)
				if flag == nil {
					t.Errorf("Expected flag %q to be added, but it was not", flagName)
					continue
				}
				if flag.Shorthand != wantShorthand {
					t.Errorf("Expected %q flag to have shorthand %q, got %q", flagName, wantShorthand, flag.Shorthand)
				}
			}
		})
	}
}

// TestGetResultsOptions tests that the GetResultsOptions function correctly retrieves flag values
func TestGetResultsOptions(t *testing.T) {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}

	// Add the flags
	AddResultsOptions(cmd)

	// Set flag values
	flagValues := map[string]string{
		kubeConfig:            "/path/to/kubeconfig",
		context:               "test-context",
		namespace:             "test-namespace",
		host:                  "https://test-host",
		token:                 "test-token",
		apiPath:               "/test/api",
		insecureSkipTLSVerify: "true",
	}

	for flagName, value := range flagValues {
		if err := cmd.PersistentFlags().Set(flagName, value); err != nil {
			t.Fatalf("Failed to set %q flag: %v", flagName, err)
		}
	}

	// Parse the flags
	if err := cmd.ParseFlags([]string{}); err != nil {
		t.Fatalf("Failed to parse flags: %v", err)
	}

	// Get the options
	opts := GetResultsOptions(cmd)

	// Check that the options match the set values
	if opts.KubeConfig != flagValues[kubeConfig] {
		t.Errorf("Expected KubeConfig to be %q, got %q", flagValues[kubeConfig], opts.KubeConfig)
	}
	if opts.Context != flagValues[context] {
		t.Errorf("Expected Context to be %q, got %q", flagValues[context], opts.Context)
	}
	if opts.Namespace != flagValues[namespace] {
		t.Errorf("Expected Namespace to be %q, got %q", flagValues[namespace], opts.Namespace)
	}
	if opts.Host != flagValues[host] {
		t.Errorf("Expected Host to be %q, got %q", flagValues[host], opts.Host)
	}
	if opts.Token != flagValues[token] {
		t.Errorf("Expected Token to be %q, got %q", flagValues[token], opts.Token)
	}
	if opts.APIPath != flagValues[apiPath] {
		t.Errorf("Expected APIPath to be %q, got %q", flagValues[apiPath], opts.APIPath)
	}
	if !opts.InsecureSkipTLSVerify {
		t.Errorf("Expected InsecureSkipTLSVerify to be true, got %v", opts.InsecureSkipTLSVerify)
	}
}

// TestInitParams tests that the InitParams function correctly initializes a Params object
func TestInitParams(t *testing.T) {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}

	// Add the flags
	AddResultsOptions(cmd)

	// Set flag values
	flagValues := map[string]string{
		kubeConfig:            "/path/to/kubeconfig",
		context:               "test-context",
		namespace:             "test-namespace",
		host:                  "https://test-host",
		token:                 "test-token",
		apiPath:               "/test/api",
		insecureSkipTLSVerify: "true",
	}

	for flagName, value := range flagValues {
		if err := cmd.PersistentFlags().Set(flagName, value); err != nil {
			t.Fatalf("Failed to set %q flag: %v", flagName, err)
		}
	}

	// Parse the flags
	if err := cmd.ParseFlags([]string{}); err != nil {
		t.Fatalf("Failed to parse flags: %v", err)
	}

	// Create a Params object
	p := &common.ResultsParams{}

	// Initialize the Params object
	err := InitParams(p, cmd)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Check that the Params object has the correct values
	if p.KubeConfigPath() != flagValues[kubeConfig] {
		t.Errorf("Expected KubeConfigPath to be %q, got %q", flagValues[kubeConfig], p.KubeConfigPath())
	}
	if p.KubeContext() != flagValues[context] {
		t.Errorf("Expected KubeContext to be %q, got %q", flagValues[context], p.KubeContext())
	}
	if p.Namespace() != flagValues[namespace] {
		t.Errorf("Expected Namespace to be %q, got %q", flagValues[namespace], p.Namespace())
	}
	if p.Host() != flagValues[host] {
		t.Errorf("Expected Host to be %q, got %q", flagValues[host], p.Host())
	}
	if p.Token() != flagValues[token] {
		t.Errorf("Expected Token to be %q, got %q", flagValues[token], p.Token())
	}
	if p.APIPath() != flagValues[apiPath] {
		t.Errorf("Expected APIPath to be %q, got %q", flagValues[apiPath], p.APIPath())
	}
	if !p.SkipTLSVerify() {
		t.Errorf("Expected SkipTLSVerify to be true, got %v", p.SkipTLSVerify())
	}
}

// TestInitParamsWithEmptyFlags tests that the InitParams function correctly handles empty flag values
func TestInitParamsWithEmptyFlags(t *testing.T) {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}

	// Add the flags
	AddResultsOptions(cmd)

	// Parse the flags
	if err := cmd.ParseFlags([]string{}); err != nil {
		t.Fatalf("Failed to parse flags: %v", err)
	}

	// Create a Params object
	p := &common.ResultsParams{}

	// Initialize the Params object
	err := InitParams(p, cmd)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}
