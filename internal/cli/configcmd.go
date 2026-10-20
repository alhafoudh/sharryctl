package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/alhafoudh/sharryctl/internal/config"
)

func (a *app) configCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "config", Short: "Manage tenants and configuration"}
	cmd.AddCommand(&cobra.Command{
		Use:   "path",
		Short: "Print the config file path",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := config.Path()
			if err != nil {
				return err
			}
			fmt.Fprintln(a.out, p)
			return nil
		},
	})

	tenants := &cobra.Command{Use: "tenants", Short: "Configured tenants"}
	tenants.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List configured tenants",
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := config.Load()
			if err != nil {
				return err
			}
			if a.g.json {
				type row struct {
					Slug     string   `json:"slug"`
					Default  bool     `json:"default"`
					Version  string   `json:"api_version,omitempty"`
					Profiles []string `json:"profiles"`
				}
				var rows []row
				for _, s := range f.Slugs() {
					p, _ := config.ListProfiles(s)
					rows = append(rows, row{s, s == f.DefaultTenant, f.Tenants[s].APIVersion, p})
				}
				return a.printJSON(rows)
			}
			var rows [][]string
			for _, s := range f.Slugs() {
				def := ""
				if s == f.DefaultTenant {
					def = "*"
				}
				p, _ := config.ListProfiles(s)
				rows = append(rows, []string{def, s, strings.Join(p, ", ")})
			}
			a.table([]string{"", "TENANT", "PROFILES"}, rows)
			return nil
		},
	})

	var t config.Tenant
	var makeDefault bool
	var fromFile string
	add := &cobra.Command{
		Use:   "add <slug>",
		Short: "Add or update a tenant (secrets are prompted if not given)",
		Long: `Add or update a tenant. The slug is the subdomain of <slug>.sharryapp.com.
If --application-token or --api-secret are omitted they are prompted without echo.
Secrets can also be supplied at runtime via SHARRY_APPLICATION_TOKEN / SHARRY_API_SECRET.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := config.Load()
			if err != nil {
				return err
			}
			slug := args[0]
			cur := f.Tenants[slug]
			if cur == nil {
				cur = &config.Tenant{}
			}
			if fromFile != "" {
				ft, err := tenantFromFile(fromFile, slug)
				if err != nil {
					return err
				}
				// Explicit flags win over the file.
				for dst, v := range map[*string]string{
					&t.ApplicationToken: ft.ApplicationToken, &t.APISecret: ft.APISecret,
					&t.APIVersion: ft.APIVersion, &t.BaseURL: ft.BaseURL, &t.UserAgent: ft.UserAgent,
					&t.AppName: ft.AppName, &t.Package: ft.Package, &t.AppVersion: ft.AppVersion,
					&t.Build: ft.Build, &t.Platform: ft.Platform, &t.Language: ft.Language,
				} {
					if *dst == "" {
						*dst = v
					}
				}
			}
			merge(&cur.APIVersion, t.APIVersion)
			merge(&cur.BaseURL, t.BaseURL)
			merge(&cur.UserAgent, t.UserAgent)
			merge(&cur.AppName, t.AppName)
			merge(&cur.Package, t.Package)
			merge(&cur.AppVersion, t.AppVersion)
			merge(&cur.Build, t.Build)
			merge(&cur.Platform, t.Platform)
			merge(&cur.Language, t.Language)
			merge(&cur.ApplicationToken, t.ApplicationToken)
			merge(&cur.APISecret, t.APISecret)
			if cur.ApplicationToken == "" {
				if cur.ApplicationToken, err = a.promptSecret("Application token: "); err != nil {
					return err
				}
			}
			if cur.APISecret == "" {
				if cur.APISecret, err = a.promptSecret("API secret: "); err != nil {
					return err
				}
			}
			f.Tenants[slug] = cur
			if makeDefault || f.DefaultTenant == "" {
				f.DefaultTenant = slug
			}
			if err := f.Save(); err != nil {
				return err
			}
			fmt.Fprintf(a.err, "Saved tenant %q.\n", slug)
			return nil
		},
	}
	fl := add.Flags()
	fl.StringVar(&t.ApplicationToken, "application-token", "", "application token (also used as client_id)")
	fl.StringVar(&t.APISecret, "api-secret", "", "API secret used for Request-Sign")
	fl.StringVar(&t.APIVersion, "api-version", "", "API version (default v8)")
	fl.StringVar(&t.BaseURL, "base-url", "", "override API base URL")
	fl.StringVar(&t.UserAgent, "user-agent", "", "override User-Agent")
	fl.StringVar(&t.AppName, "app-name", "", "app name in generated User-Agent")
	fl.StringVar(&t.Package, "package", "", "app package in generated User-Agent")
	fl.StringVar(&t.AppVersion, "app-version", "", "App-Version header")
	fl.StringVar(&t.Build, "build", "", "build number in generated User-Agent")
	fl.StringVar(&t.Platform, "platform", "", "Platform header (default Android)")
	fl.StringVar(&t.Language, "language", "", "Language header (default en)")
	fl.BoolVar(&makeDefault, "default", false, "make this the default tenant")
	fl.StringVar(&fromFile, "from-file", "", "read settings from a TOML file (flat keys, or a [tenants.<slug>] table)")

	tenants.AddCommand(add, &cobra.Command{
		Use:   "use <slug>",
		Short: "Set the default tenant",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := config.Load()
			if err != nil {
				return err
			}
			if _, ok := f.Tenants[args[0]]; !ok {
				return fmt.Errorf("tenant %q is not configured", args[0])
			}
			f.DefaultTenant = args[0]
			return f.Save()
		},
	}, &cobra.Command{
		Use:   "remove <slug>",
		Short: "Remove a tenant from config",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := config.Load()
			if err != nil {
				return err
			}
			delete(f.Tenants, args[0])
			if f.DefaultTenant == args[0] {
				f.DefaultTenant = ""
			}
			return f.Save()
		},
	}, &cobra.Command{
		Use:   "show [slug]",
		Short: "Show tenant settings (secrets masked)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := config.Load()
			if err != nil {
				return err
			}
			slug := a.g.tenant
			if len(args) == 1 {
				slug = args[0]
			}
			tn, err := f.Resolve(slug)
			if err != nil {
				return err
			}
			cfg := tn.SDKConfig()
			cfg.ApplicationToken, cfg.APISecret = mask(cfg.ApplicationToken), mask(cfg.APISecret)
			return a.printJSON(cfg)
		},
	})
	cmd.AddCommand(tenants)
	return cmd
}

// tenantFromFile reads tenant settings from a TOML file containing either flat
// keys (application_token = ...) or a [tenants.<slug>] / [<slug>] table.
func tenantFromFile(path, slug string) (*config.Tenant, error) {
	var raw map[string]any
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	table := raw
	if ts, ok := raw["tenants"].(map[string]any); ok {
		if st, ok := ts[slug].(map[string]any); ok {
			table = st
		}
	} else if st, ok := raw[slug].(map[string]any); ok {
		table = st
	}
	var t config.Tenant
	b, err := toml.Marshal(table)
	if err != nil {
		return nil, err
	}
	if err := toml.Unmarshal(b, &t); err != nil {
		return nil, err
	}
	if t.ApplicationToken == "" || t.APISecret == "" {
		return nil, fmt.Errorf("%s has no application_token / api_secret", path)
	}
	return &t, nil
}

func merge(dst *string, v string) {
	if v != "" {
		*dst = v
	}
}

func mask(s string) string {
	if len(s) <= 6 {
		return "******"
	}
	return s[:3] + strings.Repeat("*", len(s)-6) + s[len(s)-3:]
}

func (a *app) promptSecret(label string) (string, error) {
	fmt.Fprint(a.err, label)
	if term.IsTerminal(int(os.Stdin.Fd())) {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(a.err)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(b)), nil
	}
	line, err := bufio.NewReader(a.in).ReadString('\n')
	if err != nil && line == "" {
		return "", errors.New("missing secret on stdin")
	}
	return strings.TrimSpace(line), nil
}
