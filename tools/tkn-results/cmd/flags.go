package cmd

import flag "github.com/spf13/pflag"

var (
	filter    string
	limit     int32
	pageToken string
	format    string
)

func listFlags(f *flag.FlagSet) {
	f.StringVarP(&filter, "filter", "f", "", "CEL Filter")
	f.Int32VarP(&limit, "limit", "l", 0, "number of items to return. Response may be truncated due to server limits.")
	f.StringVarP(&pageToken, "page", "p", "", "pagination token to use for next page")
	f.StringVarP(&format, "output", "o", "textproto", "output format. Valid values: textproto|json")
}
