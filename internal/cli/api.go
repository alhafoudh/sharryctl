package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

func (a *app) apiCmd() *cobra.Command {
	var data string
	var query []string
	var noAuth bool
	cmd := &cobra.Command{
		Use:   "api <METHOD> <path>",
		Short: "Send a signed raw request to any endpoint",
		Long: `Send an arbitrary request through the same signing and auth as other commands.
The path is relative to the tenant API base, e.g. "parking/license-plates".

  sharryctl api GET parking/license-plates
  sharryctl api POST parking/license-plates --data '{"license_plate":"BA123AB","default":false}'
  sharryctl api GET users/42/badge-formats -q 'filter[type]=qr'`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := a.readData(data)
			if err != nil {
				return err
			}
			q := url.Values{}
			for _, kv := range query {
				k, v, ok := strings.Cut(kv, "=")
				if !ok {
					return fmt.Errorf("bad --query %q, want key=value", kv)
				}
				q.Add(k, v)
			}
			method := strings.ToUpper(args[0])
			var out json.RawMessage
			if noAuth {
				c, _, err := a.newClient()
				if err != nil {
					return err
				}
				err = c.Do(ctx(cmd), method, args[1], q, body, &out, false)
				if err != nil {
					return err
				}
			} else {
				c, _, err := a.authedClient()
				if err != nil {
					return err
				}
				if err = c.Do(ctx(cmd), method, args[1], q, body, &out, true); err != nil {
					return err
				}
			}
			return a.printJSON(out)
		},
	}
	cmd.Flags().StringVarP(&data, "data", "d", "", "request body: JSON, @file or - for stdin")
	cmd.Flags().StringArrayVarP(&query, "query", "q", nil, "query parameter key=value (repeatable)")
	cmd.Flags().BoolVar(&noAuth, "no-auth", false, "send without Access-Token (application signing only)")
	return cmd
}
