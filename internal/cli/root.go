// Package cli implements the sharryctl command tree on top of the sharry SDK.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/alhafoudh/sharryctl/internal/config"
	"github.com/alhafoudh/sharryctl/sharry"
)

// Version is set at build time via -ldflags.
var Version = "dev"

type globals struct {
	tenant  string
	profile string
	json    bool
	verbose bool
}

// app is shared state for one invocation.
type app struct {
	g   globals
	out io.Writer
	err io.Writer
	in  io.Reader
}

// NewRootCmd builds the command tree.
func NewRootCmd() *cobra.Command {
	a := &app{out: os.Stdout, err: os.Stderr, in: os.Stdin}
	root := &cobra.Command{
		Use:           "sharryctl",
		Short:         "Command-line client for the Sharry workplace platform API",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			a.out, a.err, a.in = cmd.OutOrStdout(), cmd.ErrOrStderr(), cmd.InOrStdin()
		},
	}
	pf := root.PersistentFlags()
	pf.StringVar(&a.g.tenant, "tenant", "", "tenant slug (default: config default_tenant or $SHARRY_TENANT)")
	pf.StringVar(&a.g.profile, "profile", "", "profile email to act as (default: tenant default_profile or the only logged-in profile)")
	pf.BoolVar(&a.g.json, "json", false, "machine-readable JSON output")
	pf.BoolVarP(&a.g.verbose, "verbose", "v", false, "log requests to stderr (secrets redacted)")

	root.AddCommand(
		a.loginCmd(), a.logoutCmd(), a.authCmd(), a.meCmd(),
		a.platesCmd(), a.parkingCmd(), a.badgesCmd(), a.cardFormatsCmd(), a.devicesCmd(), a.verificationCmd(),
		a.apiCmd(), a.swaggerCmd(), a.configCmd(),
	)
	return root
}

// Execute runs the CLI and returns the process exit code.
func Execute() int {
	if err := NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		var apiErr *sharry.APIError
		if errors.As(err, &apiErr) {
			return 2
		}
		return 1
	}
	return 0
}

// tenant resolves the active tenant from config.
func (a *app) tenant() (*config.Tenant, *config.File, error) {
	f, err := config.Load()
	if err != nil {
		return nil, nil, err
	}
	t, err := f.Resolve(a.g.tenant)
	return t, f, err
}

// newClient returns a client for the active tenant without user auth.
func (a *app) newClient() (*sharry.Client, *config.Tenant, error) {
	t, _, err := a.tenant()
	if err != nil {
		return nil, nil, err
	}
	cfg := t.SDKConfig()
	if a.g.verbose {
		cfg.HTTPClient = newLoggingHTTPClient(a.err)
	}
	c, err := sharry.NewClient(cfg)
	return c, t, err
}

// authedClient returns a client with the active profile's session; refreshed tokens are persisted.
func (a *app) authedClient() (*sharry.Client, *config.Profile, error) {
	c, t, err := a.newClient()
	if err != nil {
		return nil, nil, err
	}
	email := a.g.profile
	if email == "" {
		email = os.Getenv("SHARRY_PROFILE")
	}
	if email == "" {
		email = t.DefaultProfile
	}
	if email == "" {
		emails, err := config.ListProfiles(t.Slug)
		if err != nil {
			return nil, nil, err
		}
		switch len(emails) {
		case 0:
			return nil, nil, fmt.Errorf("not logged in to %q: run `sharryctl login --email <addr>`", t.Slug)
		case 1:
			email = emails[0]
		default:
			return nil, nil, fmt.Errorf("multiple profiles for %q (%s): pass --profile", t.Slug, strings.Join(emails, ", "))
		}
	}
	pr, err := config.LoadProfile(t.Slug, email)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, fmt.Errorf("no stored login for %s on %q: run `sharryctl login --email %s`", email, t.Slug, email)
		}
		return nil, nil, err
	}
	c.SetSession(sharry.NewSession(pr.Token, func(tk sharry.Token) {
		pr.Token = tk
		if err := pr.Save(); err != nil {
			fmt.Fprintln(a.err, "warning: could not persist refreshed token:", err)
		}
	}))
	return c, pr, nil
}

// currentUser returns a field ("id" or "uuid") of the cached user for default path params.
func currentUser(pr *config.Profile, field string) (string, error) {
	var u map[string]any
	if len(pr.User) == 0 || json.Unmarshal(pr.User, &u) != nil {
		return "", errors.New("no cached user in profile; pass --user or log in again")
	}
	v, ok := u[field]
	if !ok || v == nil {
		return "", fmt.Errorf("cached user has no %q; pass --user", field)
	}
	if f, ok := v.(float64); ok {
		return fmt.Sprintf("%.0f", f), nil
	}
	return fmt.Sprint(v), nil
}

// printJSON pretty-prints v (raw JSON is re-indented).
func (a *app) printJSON(v any) error {
	if raw, ok := v.(json.RawMessage); ok {
		if len(raw) == 0 {
			raw = json.RawMessage("null")
		}
		var x any
		if err := json.Unmarshal(raw, &x); err != nil {
			_, err = a.out.Write(append(raw, '\n'))
			return err
		}
		v = x
	}
	enc := json.NewEncoder(a.out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// table prints rows with a header using tabwriter.
func (a *app) table(header []string, rows [][]string) {
	w := tabwriter.NewWriter(a.out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(header, "\t"))
	for _, r := range rows {
		fmt.Fprintln(w, strings.Join(r, "\t"))
	}
	w.Flush()
}

// readData loads a --data value: literal JSON, @file, or - for stdin.
func (a *app) readData(v string) (json.RawMessage, error) {
	var data []byte
	var err error
	switch {
	case v == "":
		return nil, nil
	case v == "-":
		data, err = io.ReadAll(a.in)
	case strings.HasPrefix(v, "@"):
		data, err = os.ReadFile(v[1:])
	default:
		data = []byte(v)
	}
	if err != nil {
		return nil, err
	}
	if !json.Valid(data) {
		return nil, errors.New("--data is not valid JSON")
	}
	return json.RawMessage(data), nil
}

func ctx(cmd *cobra.Command) context.Context { return cmd.Context() }
