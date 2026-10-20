package cli

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/alhafoudh/sharryctl/internal/config"
)

// userFlag resolves --user, defaulting to a field of the cached logged-in user.
func userFlag(user string, pr *config.Profile, field string) (string, error) {
	if user != "" {
		return user, nil
	}
	return currentUser(pr, field)
}

func (a *app) badgesCmd() *cobra.Command {
	var user, data string
	cmd := &cobra.Command{Use: "badges", Short: "Access badges"}
	cmd.PersistentFlags().StringVar(&user, "user", "", "user uuid (default: logged-in user)")

	list := &cobra.Command{
		Use:   "list",
		Short: "List access badges",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, pr, err := a.authedClient()
			if err != nil {
				return err
			}
			u, err := userFlag(user, pr, "uuid")
			if err != nil {
				return err
			}
			out, err := c.ListBadges(ctx(cmd), u)
			if err != nil {
				return err
			}
			return a.printJSON(out)
		},
	}
	grant := &cobra.Command{
		Use:   "grant --data <json|@file|->",
		Short: "Grant a badge (issue-card request body)",
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := a.readData(data)
			if err != nil {
				return err
			}
			if body == nil {
				return errors.New("--data is required")
			}
			c, pr, err := a.authedClient()
			if err != nil {
				return err
			}
			u, err := userFlag(user, pr, "uuid")
			if err != nil {
				return err
			}
			out, err := c.GrantBadge(ctx(cmd), u, body)
			if err != nil {
				return err
			}
			return a.printJSON(out)
		},
	}
	grant.Flags().StringVar(&data, "data", "", "request body: JSON, @file or - for stdin")

	revoke := &cobra.Command{
		Use:   "revoke <badge>",
		Short: "Revoke a badge",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, pr, err := a.authedClient()
			if err != nil {
				return err
			}
			u, err := userFlag(user, pr, "uuid")
			if err != nil {
				return err
			}
			out, err := c.RevokeBadge(ctx(cmd), u, args[0])
			if err != nil {
				return err
			}
			return a.printJSON(out)
		},
	}
	state := &cobra.Command{
		Use:   "state <badge> <state>",
		Short: "Change a badge's state",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, pr, err := a.authedClient()
			if err != nil {
				return err
			}
			u, err := userFlag(user, pr, "uuid")
			if err != nil {
				return err
			}
			out, err := c.SetBadgeState(ctx(cmd), u, args[0], args[1])
			if err != nil {
				return err
			}
			return a.printJSON(out)
		},
	}
	cmd.AddCommand(list, grant, revoke, state)
	return cmd
}

func (a *app) cardFormatsCmd() *cobra.Command {
	var user, typ string
	var siteIDs []string
	cmd := &cobra.Command{Use: "card-formats", Short: "Badge card formats"}
	list := &cobra.Command{
		Use:   "list",
		Short: "List card formats",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, pr, err := a.authedClient()
			if err != nil {
				return err
			}
			u, err := userFlag(user, pr, "uuid")
			if err != nil {
				return err
			}
			out, err := c.ListCardFormats(ctx(cmd), u, siteIDs, typ)
			if err != nil {
				return err
			}
			return a.printJSON(out)
		},
	}
	list.Flags().StringVar(&user, "user", "", "user uuid (default: logged-in user)")
	list.Flags().StringSliceVar(&siteIDs, "site-id", nil, "filter by site id (repeatable)")
	list.Flags().StringVar(&typ, "type", "", "filter by card type")
	cmd.AddCommand(list)
	return cmd
}

func (a *app) devicesCmd() *cobra.Command {
	var user, data string
	cmd := &cobra.Command{Use: "devices", Short: "User devices"}
	state := &cobra.Command{
		Use:   "state <device> --data <json|@file|->",
		Short: "Change a device's state (change-device-state request body)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := a.readData(data)
			if err != nil {
				return err
			}
			if body == nil {
				return errors.New("--data is required")
			}
			c, pr, err := a.authedClient()
			if err != nil {
				return err
			}
			u, err := userFlag(user, pr, "uuid")
			if err != nil {
				return err
			}
			out, err := c.SetDeviceState(ctx(cmd), u, args[0], body)
			if err != nil {
				return err
			}
			return a.printJSON(out)
		},
	}
	state.Flags().StringVar(&user, "user", "", "user uuid (default: logged-in user)")
	state.Flags().StringVar(&data, "data", "", "request body: JSON, @file or - for stdin")
	cmd.AddCommand(state)
	return cmd
}

func (a *app) verificationCmd() *cobra.Command {
	var user, data string
	cmd := &cobra.Command{Use: "verification", Short: "User identity verification"}
	submit := &cobra.Command{
		Use:   "submit --data <json|@file|->",
		Short: `Submit verification; --data is the inner object, wrapped as {"verification": ...}`,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := a.readData(data)
			if err != nil {
				return err
			}
			if body == nil {
				return errors.New("--data is required")
			}
			c, pr, err := a.authedClient()
			if err != nil {
				return err
			}
			// Verification is keyed by numeric user id (users/{userId}), unlike the other endpoints.
			u, err := userFlag(user, pr, "id")
			if err != nil {
				return err
			}
			out, err := c.SubmitVerification(ctx(cmd), u, body)
			if err != nil {
				return err
			}
			return a.printJSON(out)
		},
	}
	submit.Flags().StringVar(&user, "user", "", "numeric user id (default: logged-in user)")
	submit.Flags().StringVar(&data, "data", "", "verification object: JSON, @file or - for stdin")
	cmd.AddCommand(submit)
	return cmd
}
