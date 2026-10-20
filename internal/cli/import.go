package cli

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/alhafoudh/sharryctl/internal/config"
	"github.com/alhafoudh/sharryctl/sharry"
)

func (a *app) authImportCmd() *cobra.Command {
	var email, file string
	cmd := &cobra.Command{
		Use:   "import --file <tokens.json|-> [--email <addr>]",
		Short: "Store existing session tokens without the email-code login",
		Long: `Imports access_token / refresh_token (and optionally expires, email, user) from a
JSON file, anywhere in its structure, into the profile store (mode 0600).
If expires is missing it is taken from the JWT exp claim. Token values are never printed.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if file == "" {
				return errors.New("--file is required")
			}
			var data []byte
			var err error
			if file == "-" {
				data, err = io.ReadAll(a.in)
			} else {
				data, err = os.ReadFile(file)
			}
			if err != nil {
				return err
			}
			var doc any
			if err := json.Unmarshal(data, &doc); err != nil {
				return fmt.Errorf("parse %s: %w", file, err)
			}
			found := map[string]any{}
			collect(doc, found)

			access, _ := found["access_token"].(string)
			refresh, _ := found["refresh_token"].(string)
			if access == "" && refresh == "" {
				return errors.New("no access_token or refresh_token found in file")
			}
			tk := sharry.Token{AccessToken: access, RefreshToken: refresh}
			if s, ok := found["expires"].(string); ok {
				if t, err := time.Parse(time.RFC3339, s); err == nil {
					tk.Expires = t
				}
			}
			if tk.Expires.IsZero() {
				tk.Expires = jwtExpiry(access)
			}

			var user json.RawMessage
			if u, ok := found["user"].(map[string]any); ok {
				user, _ = json.Marshal(u)
				if email == "" {
					email, _ = u["email"].(string)
				}
			}
			if email == "" {
				email, _ = found["email"].(string)
			}
			if email == "" {
				return errors.New("no email in file: pass --email")
			}

			slug, err := a.tenantSlug()
			if err != nil {
				return err
			}
			pr := &config.Profile{Tenant: slug, Email: email, Token: tk, User: user}
			if old, err := config.LoadProfile(slug, email); err == nil && len(user) == 0 {
				pr.User = old.User
			}
			if err := pr.Save(); err != nil {
				return err
			}
			exp := "unknown"
			if !tk.Expires.IsZero() {
				exp = tk.Expires.Local().Format(time.RFC1123)
			}
			fmt.Fprintf(a.err, "Imported tokens for %s on %q (access: %t, refresh: %t, expires %s, cached user: %t).\n",
				email, slug, access != "", refresh != "", exp, len(pr.User) > 0)

			// Pick up tenant app credentials too when the file carries them.
			appToken, _ := found["application_token"].(string)
			if appToken == "" {
				appToken, _ = found["client_id"].(string)
			}
			apiSecret, _ := found["api_secret"].(string)
			f, err := config.Load()
			if err != nil {
				return err
			}
			t := f.Tenants[slug]
			if t == nil {
				t = &config.Tenant{}
			}
			if appToken != "" && apiSecret != "" {
				t.ApplicationToken, t.APISecret = appToken, apiSecret
				f.Tenants[slug] = t
				if f.DefaultTenant == "" {
					f.DefaultTenant = slug
				}
				if err := f.Save(); err != nil {
					return err
				}
				fmt.Fprintf(a.err, "Saved application token and API secret for tenant %q.\n", slug)
			} else if t.ApplicationToken == "" || t.APISecret == "" {
				fmt.Fprintf(a.err, "warning: tenant %q has no application token / API secret yet; requests will fail until you run:\n  sharryctl config tenants add %s\n", slug, slug)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "profile email (default: from file)")
	cmd.Flags().StringVar(&file, "file", "", "JSON file with tokens, or - for stdin")
	return cmd
}

// tenantSlug resolves the tenant slug without requiring its secrets to be configured.
func (a *app) tenantSlug() (string, error) {
	if a.g.tenant != "" {
		return a.g.tenant, nil
	}
	if s := os.Getenv("SHARRY_TENANT"); s != "" {
		return s, nil
	}
	f, err := config.Load()
	if err != nil {
		return "", err
	}
	if f.DefaultTenant != "" {
		return f.DefaultTenant, nil
	}
	return "", errors.New("no tenant selected: pass --tenant")
}

// collect records the first occurrence of interesting keys anywhere in doc.
func collect(doc any, found map[string]any) {
	switch v := doc.(type) {
	case map[string]any:
		for k, val := range v {
			switch k {
			case "access_token", "refresh_token", "expires", "email", "user",
				"application_token", "api_secret", "client_id":
				if _, seen := found[k]; !seen {
					found[k] = val
				}
			}
		}
		for k, val := range v {
			if k != "user" {
				collect(val, found)
			}
		}
	case []any:
		for _, val := range v {
			collect(val, found)
		}
	}
}

// jwtExpiry reads the exp claim of a JWT without verifying it.
func jwtExpiry(tok string) time.Time {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return time.Time{}
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}
	}
	var claims struct {
		Exp float64 `json:"exp"`
	}
	if json.Unmarshal(payload, &claims) != nil || claims.Exp == 0 {
		return time.Time{}
	}
	return time.Unix(int64(claims.Exp), 0)
}
