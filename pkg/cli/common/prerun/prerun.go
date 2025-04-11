package prerun

import (
	"github.com/spf13/cobra"
	"github.com/tektoncd/results/pkg/cli/common"
	"github.com/tektoncd/results/pkg/cli/flags"
)

// PersistentPreRunE returns a function that can be used as a persistent pre-run
// function for Cobra commands. It initializes the provided parameters using
// the flags defined in the command.
//
// Parameters:
//   - p: A common.Params struct that will be initialized with values from command flags.
//
// Returns:
//   - A function that takes a *cobra.Command and a []string, and returns an error.
//     This function initializes the params using flags.InitParams and returns any error encountered.
func PersistentPreRunE(p common.Params) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		return flags.InitParams(p, cmd)
	}
}
