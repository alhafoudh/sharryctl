package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/alhafoudh/sharryctl/internal/config"
	"github.com/alhafoudh/sharryctl/sharry"
)

func (a *app) loginCmd() *cobra.Command {
	var email, code, companyID, locationID string
	var agree, skipRequest, requestOnly bool
	cmd := &cobra.Command{
		Use:   "login --email <addr>",
		Short: "Log in with an emailed one-time code",
		Long: `Runs the identity check, requests a login code to be emailed, then submits it.

The code is read from --code, from stdin with --code -, or prompted interactively.

Non-interactive two-step flow (scripts, agents):
  sharryctl login --email you@example.com --request-only
  sharryctl login --email you@example.com --skip-request --code 123456`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if email == "" {
				return errors.New("--email is required")
			}
			if skipRequest && requestOnly {
				return errors.New("--skip-request and --request-only are mutually exclusive")
			}
			c, t, err := a.newClient()
			if err != nil {
				return err
			}
			if !skipRequest {
				id, err := c.CheckUser(ctx(cmd), email)
				if err != nil {
					return fmt.Errorf("identity check: %w", err)
				}
				if id.SSOLink != nil && *id.SSOLink != "" {
					fmt.Fprintf(a.err, "note: this account uses SSO (%v): %s\n", deref(id.SSOProvider), *id.SSOLink)
				}
				opts := sharry.CodeChallengeOptions{LocationID: id.LocationID}
				if locationID != "" {
					opts.LocationID = locationID
				}
				if companyID != "" {
					opts.CompanyID = companyID
				}
				if cmd.Flags().Changed("agree-privacy") {
					opts.AgreeWithPrivacy = &agree
				}
				if _, err := c.RequestCode(ctx(cmd), email, opts); err != nil {
					return fmt.Errorf("request code: %w", err)
				}
				fmt.Fprintf(a.err, "A login code was sent to %s.\n", email)
				if requestOnly {
					return nil
				}
			}
			if code, err = a.obtainCode(code); err != nil {
				return err
			}
			login, err := c.SubmitCode(ctx(cmd), email, code)
			if err != nil {
				return fmt.Errorf("submit code: %w", err)
			}
			user, _ := json.Marshal(login.User)
			pr := &config.Profile{Tenant: t.Slug, Email: email, User: user, Token: c.Session().Token()}
			if err := pr.Save(); err != nil {
				return err
			}
			if a.g.json {
				return a.printJSON(map[string]any{"tenant": t.Slug, "email": email, "expires": login.Expires, "user": login.User})
			}
			fmt.Fprintf(a.out, "Logged in to %s as %s (%s). Token expires %s.\n",
				t.Slug, login.User.Fullname, email, login.Expires.Local().Format(time.RFC1123))
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&email, "email", "", "account email")
	f.StringVar(&code, "code", "", "login code, or - to read it from stdin")
	f.StringVar(&companyID, "company-id", "", "company to log in to (multi-company accounts)")
	f.StringVar(&locationID, "location-id", "", "location override")
	f.BoolVar(&agree, "agree-privacy", false, "send agree_with_privacy")
	f.BoolVar(&skipRequest, "skip-request", false, "do not request a new code, only submit --code")
	f.BoolVar(&requestOnly, "request-only", false, "only request the emailed code, then exit")

	resend := &cobra.Command{
		Use:   "resend --email <addr>",
		Short: "Resend the login code",
		RunE: func(cmd *cobra.Command, args []string) error {
			if email == "" {
				return errors.New("--email is required")
			}
			c, _, err := a.newClient()
			if err != nil {
				return err
			}
			if err := c.ResendCode(ctx(cmd), email); err != nil {
				return err
			}
			fmt.Fprintf(a.err, "Code re-sent to %s.\n", email)
			return nil
		},
	}
	resend.Flags().StringVar(&email, "email", "", "account email")
	cmd.AddCommand(resend)
	return cmd
}

func (a *app) obtainCode(code string) (string, error) {
	if code == "-" {
		line, err := bufio.NewReader(a.in).ReadString('\n')
		if err != nil && line == "" {
			return "", fmt.Errorf("read code from stdin: %w", err)
		}
		code = line
	}
	if code == "" {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return "", errors.New("no --code given and stdin is not a terminal")
		}
		fmt.Fprint(a.err, "Enter code: ")
		line, err := bufio.NewReader(a.in).ReadString('\n')
		if err != nil && line == "" {
			return "", err
		}
		code = line
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return "", errors.New("empty code")
	}
	return code, nil
}

func (a *app) logoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Forget the stored tokens of the active profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, pr, err := a.authedClient()
			if err != nil {
				return err
			}
			if err := config.DeleteProfile(pr.Tenant, pr.Email); err != nil {
				return err
			}
			fmt.Fprintf(a.err, "Logged out %s from %s.\n", pr.Email, pr.Tenant)
			return nil
		},
	}
}

func (a *app) authCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "auth", Short: "Inspect and manage stored credentials"}
	cmd.AddCommand(a.authImportCmd(), &cobra.Command{
		Use:   "status",
		Short: "Show the active profile and token expiry",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, pr, err := a.authedClient()
			if err != nil {
				return err
			}
			tk := pr.Token
			st := map[string]any{
				"tenant": pr.Tenant, "email": pr.Email, "expires": tk.Expires,
				"expired": tk.Expired(0), "has_refresh_token": tk.RefreshToken != "",
			}
			if a.g.json {
				return a.printJSON(st)
			}
			state := "valid"
			if tk.Expired(0) {
				state = "expired (will refresh on next call)"
			}
			fmt.Fprintf(a.out, "Tenant:  %s\nProfile: %s\nToken:   %s, expires %s\n",
				pr.Tenant, pr.Email, state, tk.Expires.Local().Format(time.RFC1123))
			return nil
		},
	}, &cobra.Command{
		Use:   "refresh",
		Short: "Force a token refresh",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := a.authedClient()
			if err != nil {
				return err
			}
			r, err := c.Refresh(ctx(cmd))
			if err != nil {
				return err
			}
			if a.g.json {
				return a.printJSON(map[string]any{"expires": r.Expires})
			}
			fmt.Fprintf(a.out, "Refreshed. Token expires %s.\n", r.Expires.Local().Format(time.RFC1123))
			return nil
		},
	}, &cobra.Command{
		Use:   "token",
		Short: "Print a valid access token (refreshing if needed)",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := a.authedClient()
			if err != nil {
				return err
			}
			tok, err := c.AccessToken(ctx(cmd))
			if err != nil {
				return err
			}
			fmt.Fprintln(a.out, tok)
			return nil
		},
	})
	return cmd
}

func (a *app) meCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "me",
		Short: "Show the logged-in user's profile (GET users/me, cached in the profile)",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, pr, err := a.authedClient()
			if err != nil {
				return err
			}
			u, raw, err := c.Me(ctx(cmd))
			if err != nil {
				return err
			}
			pr.User = raw
			if err := pr.Save(); err != nil {
				fmt.Fprintln(a.err, "warning: could not cache profile:", err)
			}
			if a.g.json {
				return a.printJSON(raw)
			}
			company := ""
			if u.Company != nil {
				company = u.Company.Name
			}
			fmt.Fprintf(a.out, "Name:    %s\nEmail:   %s\nID:      %v\nUUID:    %s\nCompany: %s\nStatus:  %s\n",
				u.Fullname, u.Email, u.ID, u.UUID, company, u.Status)
			return nil
		},
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
