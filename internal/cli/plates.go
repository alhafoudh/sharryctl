package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alhafoudh/sharryctl/sharry"
)

func (a *app) platesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "plates",
		Aliases: []string{"plate", "license-plates"},
		Short:   "Manage parking license plates",
	}

	list := &cobra.Command{
		Use:   "list",
		Short: "List your license plates",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := a.authedClient()
			if err != nil {
				return err
			}
			plates, err := c.ListLicensePlates(ctx(cmd))
			if err != nil {
				return err
			}
			return a.printPlates(plates)
		},
	}

	var setDefault bool
	add := &cobra.Command{
		Use:   "add <plate>",
		Short: "Register a new license plate",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := a.authedClient()
			if err != nil {
				return err
			}
			p, err := c.AddLicensePlate(ctx(cmd), sharry.LicensePlateRequest{
				LicensePlate: normalizePlate(args[0]), Default: &setDefault,
			})
			if err != nil {
				return err
			}
			return a.printPlate(p, "Added")
		},
	}
	add.Flags().BoolVar(&setDefault, "default", false, "make it the default plate")

	var newNumber string
	var changeDefault bool
	change := &cobra.Command{
		Use:     "change <uuid|plate> [--to <new plate>] [--default]",
		Aliases: []string{"edit", "update"},
		Short:   "Change a plate's number and/or make it the default",
		Long: `Change a registered plate. Identify it by uuid or by its current number.

  sharryctl plates change BA123AB --to BA456CD
  sharryctl plates change BA456CD --default`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if newNumber == "" && !cmd.Flags().Changed("default") {
				return errors.New("nothing to change: pass --to and/or --default")
			}
			c, _, err := a.authedClient()
			if err != nil {
				return err
			}
			cur, err := resolvePlate(cmd, c, args[0])
			if err != nil {
				return err
			}
			// Send the full object: the app always sends both fields.
			req := sharry.LicensePlateRequest{LicensePlate: cur.LicensePlate, Default: cur.Default}
			if newNumber != "" {
				req.LicensePlate = normalizePlate(newNumber)
			}
			if cmd.Flags().Changed("default") {
				req.Default = &changeDefault
			}
			p, err := c.UpdateLicensePlate(ctx(cmd), cur.UUID, req)
			if err != nil {
				return err
			}
			return a.printPlate(p, "Changed")
		},
	}
	change.Flags().StringVar(&newNumber, "to", "", "new plate number")
	change.Flags().BoolVar(&changeDefault, "default", false, "set (or with --default=false unset) as default")

	remove := &cobra.Command{
		Use:     "remove <uuid|plate>",
		Aliases: []string{"rm", "delete"},
		Short:   "Delete a license plate",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := a.authedClient()
			if err != nil {
				return err
			}
			cur, err := resolvePlate(cmd, c, args[0])
			if err != nil {
				return err
			}
			if err := c.DeleteLicensePlate(ctx(cmd), cur.UUID); err != nil {
				return err
			}
			if a.g.json {
				return a.printJSON(map[string]any{"deleted": true, "uuid": cur.UUID})
			}
			fmt.Fprintf(a.out, "Removed %s (%s).\n", cur.LicensePlate, cur.UUID)
			return nil
		},
	}

	cmd.AddCommand(list, add, change, remove)
	return cmd
}

// resolvePlate finds a plate by uuid or plate number.
func resolvePlate(cmd *cobra.Command, c *sharry.Client, ref string) (*sharry.LicensePlate, error) {
	plates, err := c.ListLicensePlates(ctx(cmd))
	if err != nil {
		return nil, err
	}
	norm := normalizePlate(ref)
	for i := range plates {
		if plates[i].UUID == ref || normalizePlate(plates[i].LicensePlate) == norm {
			return &plates[i], nil
		}
	}
	return nil, fmt.Errorf("no license plate matches %q", ref)
}

// normalizePlate uppercases and strips spaces and dashes.
func normalizePlate(s string) string {
	return strings.NewReplacer(" ", "", "-", "").Replace(strings.ToUpper(strings.TrimSpace(s)))
}

func (a *app) printPlates(plates []sharry.LicensePlate) error {
	if a.g.json {
		if plates == nil {
			plates = []sharry.LicensePlate{}
		}
		return a.printJSON(plates)
	}
	rows := make([][]string, 0, len(plates))
	for _, p := range plates {
		def := ""
		if p.Default != nil && *p.Default {
			def = "yes"
		}
		rows = append(rows, []string{p.LicensePlate, def, p.UUID})
	}
	a.table([]string{"PLATE", "DEFAULT", "UUID"}, rows)
	return nil
}

func (a *app) printPlate(p *sharry.LicensePlate, verb string) error {
	if a.g.json {
		return a.printJSON(p)
	}
	if p == nil {
		fmt.Fprintf(a.out, "%s.\n", verb)
		return nil
	}
	def := ""
	if p.Default != nil && *p.Default {
		def = " (default)"
	}
	fmt.Fprintf(a.out, "%s %s%s [%s].\n", verb, p.LicensePlate, def, p.UUID)
	return nil
}
