package cmd

import (
	"github.com/spf13/cobra"
)

var (
	recordsCmd = &cobra.Command{
		Use:   "records",
		Short: "Command sub-group for querying Records",
	}
)

func init() {
	RootCmd.AddCommand(recordsCmd)
}
