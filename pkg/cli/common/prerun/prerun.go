package prerun

import (
	"github.com/spf13/cobra"
	"github.com/tektoncd/results/pkg/cli/common"
	"github.com/tektoncd/results/pkg/cli/flags"
)

func PersistentPreRunE(p common.Params) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		return flags.InitParams(p, cmd)
	}
}
