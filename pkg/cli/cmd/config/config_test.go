package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tektoncd/results/pkg/cli/common"
	"github.com/tektoncd/results/pkg/cli/config/test"
)

func TestCommand(t *testing.T) {
	tests := []struct {
		name            string
		params          common.Params
		wantErr         bool
		expectedUse     string
		expectedShort   string
		expectedFlags   []string
		expectedSubcmds []string
	}{
		{
			name:          "valid params",
			params:        &test.Params{},
			wantErr:       false,
			expectedUse:   "config",
			expectedShort: "Manage Tekton Results CLI configuration",
			expectedFlags: []string{
				"kubeconfig",
				"context",
				"namespace",
				"host",
				"token",
				"api-path",
				"insecure-skip-tls-verify",
			},
			expectedSubcmds: []string{"set", "reset", "view"},
		},
		{
			name:          "nil params",
			params:        nil,
			wantErr:       false,
			expectedUse:   "config",
			expectedShort: "Manage Tekton Results CLI configuration",
			expectedFlags: []string{
				"kubeconfig",
				"context",
				"namespace",
				"host",
				"token",
				"api-path",
				"insecure-skip-tls-verify",
			},
			expectedSubcmds: []string{"set", "reset", "view"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := Command(tt.params)
			if tt.wantErr {
				assert.Nil(t, cmd)
				return
			}

			require.NotNil(t, cmd)
			assert.Equal(t, tt.expectedUse, cmd.Use)
			assert.Equal(t, tt.expectedShort, cmd.Short)

			// Verify flags
			for _, flagName := range tt.expectedFlags {
				flag := cmd.PersistentFlags().Lookup(flagName)
				assert.NotNil(t, flag, "Expected flag %s to exist", flagName)
			}

			// Verify subcommands
			subcmds := cmd.Commands()
			assert.Len(t, subcmds, len(tt.expectedSubcmds))
			for _, expectedSubcmd := range tt.expectedSubcmds {
				found := false
				for _, subcmd := range subcmds {
					if subcmd.Use == expectedSubcmd {
						found = true
						// Verify subcommand properties
						assert.NotNil(t, subcmd.RunE, "Subcommand %s should have a RunE function", expectedSubcmd)
						assert.NotEmpty(t, subcmd.Short, "Subcommand %s should have a Short description", expectedSubcmd)
						break
					}
				}
				assert.True(t, found, "Expected subcommand %s to exist", expectedSubcmd)
			}
		})
	}
}

func TestCommandPersistentPreRunE(t *testing.T) {
	tests := []struct {
		name    string
		params  common.Params
		wantErr bool
	}{
		{
			name:    "valid params",
			params:  &test.Params{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := Command(tt.params)
			// Command should always be created, even with nil params
			require.NotNil(t, cmd, "Command should not be nil even for nil params")
			require.NotNil(t, cmd.PersistentPreRunE, "PersistentPreRunE should not be nil")

			// Parse flags before running PersistentPreRunE
			cmd.SetArgs([]string{})
			if err := cmd.ParseFlags([]string{}); err != nil {
				t.Fatalf("Failed to parse flags: %v", err)
			}

			// Only test the PersistentPreRunE function if we have a command
			err := cmd.PersistentPreRunE(cmd, []string{})
			if tt.wantErr {
				assert.Error(t, err, "Expected error for nil params")
			} else {
				assert.NoError(t, err, "Expected no error for valid params")
			}
		})
	}
}

func TestCommandFlagDefaults(t *testing.T) {
	params := &test.Params{}
	cmd := Command(params)

	// Test flag default values
	tests := []struct {
		name         string
		flagName     string
		defaultValue string
	}{
		{
			name:         "kubeconfig default",
			flagName:     "kubeconfig",
			defaultValue: "",
		},
		{
			name:         "context default",
			flagName:     "context",
			defaultValue: "",
		},
		{
			name:         "namespace default",
			flagName:     "namespace",
			defaultValue: "",
		},
		{
			name:         "host default",
			flagName:     "host",
			defaultValue: "",
		},
		{
			name:         "token default",
			flagName:     "token",
			defaultValue: "",
		},
		{
			name:         "api-path default",
			flagName:     "api-path",
			defaultValue: "",
		},
		{
			name:         "insecure-skip-tls-verify default",
			flagName:     "insecure-skip-tls-verify",
			defaultValue: "false",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := cmd.PersistentFlags().Lookup(tt.flagName)
			require.NotNil(t, flag)
			assert.Equal(t, tt.defaultValue, flag.DefValue)
		})
	}
}

func TestCommandFlagShorthands(t *testing.T) {
	params := &test.Params{}
	cmd := Command(params)

	// Test flag shorthand values
	tests := []struct {
		name      string
		flagName  string
		shorthand string
	}{
		{
			name:      "kubeconfig shorthand",
			flagName:  "kubeconfig",
			shorthand: "k",
		},
		{
			name:      "context shorthand",
			flagName:  "context",
			shorthand: "c",
		},
		{
			name:      "namespace shorthand",
			flagName:  "namespace",
			shorthand: "n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := cmd.PersistentFlags().Lookup(tt.flagName)
			require.NotNil(t, flag)
			assert.Equal(t, tt.shorthand, flag.Shorthand)
		})
	}
}
