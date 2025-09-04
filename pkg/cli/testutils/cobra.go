package testutils

import (
	"bytes"

	"github.com/spf13/cobra"
)

// ExecuteCommand executes the root command passing the args and returns
// the output as a string and error
func ExecuteCommand(c *cobra.Command, args ...string) (string, error) {
	buf := new(bytes.Buffer)
	c.SetOut(buf)
	c.SetErr(buf)
	c.SetArgs(args)
	c.SilenceUsage = true

	_, err := c.ExecuteC()

	return buf.String(), err
}
