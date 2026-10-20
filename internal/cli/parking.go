package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/alhafoudh/sharryctl/internal/config"
	"github.com/alhafoudh/sharryctl/sharry"
)

// identity returns the logged-in user's uuid and company uuid, fetching and
// caching users/me when the profile has no cached user yet.
func (a *app) identity(cmd *cobra.Command, c *sharry.Client, pr *config.Profile) (userUUID, companyUUID string, err error) {
	var u sharry.User
	if len(pr.User) == 0 || json.Unmarshal(pr.User, &u) != nil || u.UUID == "" || u.Company == nil {
		me, raw, err := c.Me(ctx(cmd))
		if err != nil {
			return "", "", fmt.Errorf("fetch profile: %w", err)
		}
		pr.User = raw
		_ = pr.Save()
		u = *me
	}
	if u.Company == nil || u.Company.UUID == "" {
		return u.UUID, "", errors.New("profile has no company; pass --company")
	}
	return u.UUID, u.Company.UUID, nil
}

func (a *app) parkingCmd() *cobra.Command {
	var company string
	cmd := &cobra.Command{Use: "parking", Short: "Parking availability and reservations"}
	cmd.PersistentFlags().StringVar(&company, "company", "", "company uuid (default: your company)")

	companyOf := func(cmd *cobra.Command, c *sharry.Client, pr *config.Profile) (string, string, error) {
		user, comp, err := a.identity(cmd, c, pr)
		if company != "" {
			return user, company, nil
		}
		return user, comp, err
	}

	status := &cobra.Command{
		Use:   "status",
		Short: "Free, reserved and occupied spots for your company and the site",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, pr, err := a.authedClient()
			if err != nil {
				return err
			}
			_, comp, err := companyOf(cmd, c, pr)
			if err != nil {
				return err
			}
			st, err := c.ParkingStatus(ctx(cmd), comp)
			if err != nil {
				return err
			}
			if a.g.json {
				return a.printJSON(st)
			}
			var rows [][]string
			for _, co := range st.Companies {
				s := co.SpotsCount
				rows = append(rows, []string{co.Name, itoa(s.Total), itoa(s.Free), itoa(s.Reserved), itoa(s.Occupied),
					fmt.Sprintf("%d guaranteed, %d bookable, %d visitor", co.GuaranteedSpots, co.SpotsAllowed.Bookable, co.SpotsAllowed.Visitor)})
			}
			s := st.SpotsCount
			rows = append(rows, []string{"(whole site)", itoa(s.Total), itoa(s.Free), itoa(s.Reserved), itoa(s.Occupied),
				fmt.Sprintf("public: %d free, %d reserved", s.PublicFree, s.PublicReserved)})
			a.table([]string{"SCOPE", "TOTAL", "FREE", "RESERVED", "OCCUPIED", "ALLOCATION"}, rows)
			return nil
		},
	}

	list := &cobra.Command{
		Use:     "reservations",
		Aliases: []string{"list", "ls"},
		Short:   "List your parking reservations",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, pr, err := a.authedClient()
			if err != nil {
				return err
			}
			user, _, err := a.identity(cmd, c, pr)
			if err != nil && user == "" {
				return err
			}
			res, err := c.ListParkingReservations(ctx(cmd), user)
			if err != nil {
				return err
			}
			return a.printReservations(res)
		},
	}

	var from, to string
	recommend := &cobra.Command{
		Use:   "recommend --from <time> --to <time>",
		Short: "Spots you can book for a time window",
		Long:  "Times accept RFC 3339, \"2026-10-21 09:00\", \"09:00\" (today), \"tomorrow 09:00\" or \"now\".",
		RunE: func(cmd *cobra.Command, args []string) error {
			f, t, err := parseWindow(from, to)
			if err != nil {
				return err
			}
			c, pr, err := a.authedClient()
			if err != nil {
				return err
			}
			_, comp, err := companyOf(cmd, c, pr)
			if err != nil {
				return err
			}
			spots, err := c.ParkingRecommendations(ctx(cmd), comp, f, t)
			if err != nil {
				return err
			}
			if a.g.json {
				if spots == nil {
					spots = []sharry.ParkingSpot{}
				}
				return a.printJSON(spots)
			}
			if len(spots) == 0 {
				fmt.Fprintf(a.out, "No spots available %s.\n", window(f, t))
				return nil
			}
			var rows [][]string
			for _, s := range spots {
				rows = append(rows, []string{fmt.Sprint(s.Number), s.Label, s.Type, s.Status, s.UUID})
			}
			a.table([]string{"NUMBER", "LABEL", "TYPE", "STATUS", "UUID"}, rows)
			return nil
		},
	}
	recommend.Flags().StringVar(&from, "from", "", "window start")
	recommend.Flags().StringVar(&to, "to", "", "window end")

	var spot string
	reserve := &cobra.Command{
		Use:   "reserve --from <time> --to <time> [--spot <uuid>]",
		Short: "Book a parking spot (first recommended spot unless --spot is given)",
		Long: `Books a real parking spot. Without --spot the first recommended spot is used.
The reservation uses your default license plate (see "sharryctl plates").
Times accept RFC 3339, "2026-10-21 09:00", "09:00" (today), "tomorrow 09:00" or "now".`,
		RunE: func(cmd *cobra.Command, args []string) error {
			f, t, err := parseWindow(from, to)
			if err != nil {
				return err
			}
			c, pr, err := a.authedClient()
			if err != nil {
				return err
			}
			user, comp, err := companyOf(cmd, c, pr)
			if err != nil {
				return err
			}
			if spot == "" {
				spots, err := c.ParkingRecommendations(ctx(cmd), comp, f, t)
				if err != nil {
					return err
				}
				if len(spots) == 0 {
					return fmt.Errorf("no spots available %s", window(f, t))
				}
				spot = spots[0].UUID
				fmt.Fprintf(a.err, "Using recommended spot %v (%s).\n", spots[0].Number, spot)
			}
			r, err := c.CreateParkingReservation(ctx(cmd), user, spot, f, t)
			if err != nil {
				return err
			}
			if a.g.json {
				return a.printJSON(r)
			}
			fmt.Fprintf(a.out, "Reserved spot %v %s [%s].\n", spotNumber(r), window(f, t), r.UUID)
			return nil
		},
	}
	reserve.Flags().StringVar(&from, "from", "", "start")
	reserve.Flags().StringVar(&to, "to", "", "end")
	reserve.Flags().StringVar(&spot, "spot", "", "spot uuid (default: first recommendation)")

	cancel := &cobra.Command{
		Use:   "cancel <reservation-uuid>",
		Short: "Cancel a reservation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, pr, err := a.authedClient()
			if err != nil {
				return err
			}
			user, _, err := a.identity(cmd, c, pr)
			if err != nil && user == "" {
				return err
			}
			if err := c.CancelParkingReservation(ctx(cmd), user, args[0]); err != nil {
				return err
			}
			if a.g.json {
				return a.printJSON(map[string]any{"cancelled": true, "uuid": args[0]})
			}
			fmt.Fprintf(a.out, "Cancelled %s.\n", args[0])
			return nil
		},
	}

	var relFrom, relTo string
	release := &cobra.Command{
		Use:   "release <reservation-uuid> [--from <time>] [--to <time>]",
		Short: "Free your spot for part of a reservation (default: from now to its end)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, pr, err := a.authedClient()
			if err != nil {
				return err
			}
			user, _, err := a.identity(cmd, c, pr)
			if err != nil && user == "" {
				return err
			}
			if relFrom == "" {
				relFrom = "now"
			}
			f, err := parseWhen(relFrom, time.Now())
			if err != nil {
				return err
			}
			var t time.Time
			if relTo != "" {
				if t, err = parseWhen(relTo, time.Now()); err != nil {
					return err
				}
			} else {
				res, err := c.ListParkingReservations(ctx(cmd), user)
				if err != nil {
					return err
				}
				for _, r := range res {
					if r.UUID == args[0] {
						t, err = time.Parse(time.RFC3339, r.End)
						if err != nil {
							return fmt.Errorf("cannot parse reservation end %q; pass --to", r.End)
						}
					}
				}
				if t.IsZero() {
					return fmt.Errorf("reservation %s not found; pass --to", args[0])
				}
			}
			r, err := c.ReleaseParkingReservation(ctx(cmd), user, args[0], f, t)
			if err != nil {
				return err
			}
			if a.g.json {
				return a.printJSON(r)
			}
			fmt.Fprintf(a.out, "Released spot %s.\n", window(f, t))
			return nil
		},
	}
	release.Flags().StringVar(&relFrom, "from", "", "release start (default now)")
	release.Flags().StringVar(&relTo, "to", "", "release end (default: reservation end)")

	cmd.AddCommand(status, list, recommend, reserve, cancel, release)
	return cmd
}

func (a *app) printReservations(res []sharry.ParkingReservation) error {
	if a.g.json {
		if res == nil {
			res = []sharry.ParkingReservation{}
		}
		return a.printJSON(res)
	}
	if len(res) == 0 {
		fmt.Fprintln(a.out, "No parking reservations.")
		return nil
	}
	var rows [][]string
	for _, r := range res {
		plate := ""
		if r.LicensePlate != nil {
			plate = *r.LicensePlate
		}
		rows = append(rows, []string{localTime(r.Start), localTime(r.End), fmt.Sprint(spotNumber(&r)), plate, r.ReservationType, r.UUID})
	}
	a.table([]string{"START", "END", "SPOT", "PLATE", "TYPE", "UUID"}, rows)
	return nil
}

func spotNumber(r *sharry.ParkingReservation) any {
	if r.SpotNumber != nil {
		return r.SpotNumber
	}
	if r.Spot != nil {
		return r.Spot.Number
	}
	return "?"
}

func parseWindow(from, to string) (time.Time, time.Time, error) {
	if from == "" || to == "" {
		return time.Time{}, time.Time{}, errors.New("--from and --to are required")
	}
	now := time.Now()
	f, err := parseWhen(from, now)
	if err != nil {
		return f, f, err
	}
	t, err := parseWhen(to, f)
	if err != nil {
		return f, t, err
	}
	if !t.After(f) {
		return f, t, errors.New("--to must be after --from")
	}
	return f, t, nil
}

// parseWhen parses a user-friendly time in the local zone. A bare "HH:MM" is
// taken on the same day as ref (so --to 17:00 follows --from tomorrow 09:00).
func parseWhen(s string, ref time.Time) (time.Time, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "now" {
		return time.Now(), nil
	}
	if t, err := time.Parse(time.RFC3339, strings.ToUpper(s)); err == nil {
		return t, nil
	}
	for _, layout := range []string{"2006-01-02 15:04", "2006-01-02t15:04", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, nil
		}
	}
	day := ref
	switch {
	case strings.HasPrefix(s, "tomorrow"):
		day = time.Now().AddDate(0, 0, 1)
		s = strings.TrimSpace(strings.TrimPrefix(s, "tomorrow"))
	case strings.HasPrefix(s, "today"):
		day = time.Now()
		s = strings.TrimSpace(strings.TrimPrefix(s, "today"))
	}
	if hm, err := time.Parse("15:04", s); err == nil {
		return time.Date(day.Year(), day.Month(), day.Day(), hm.Hour(), hm.Minute(), 0, 0, time.Local), nil
	}
	return time.Time{}, fmt.Errorf("cannot parse time %q (try \"tomorrow 09:00\" or 2026-10-21 09:00)", s)
}

func window(f, t time.Time) string {
	if f.YearDay() == t.YearDay() && f.Year() == t.Year() {
		return fmt.Sprintf("on %s %s-%s", f.Format("Mon 2 Jan"), f.Format("15:04"), t.Format("15:04"))
	}
	return fmt.Sprintf("from %s to %s", f.Format("Mon 2 Jan 15:04"), t.Format("Mon 2 Jan 15:04"))
}

func localTime(s string) string {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Local().Format("Mon 2 Jan 15:04")
		}
	}
	return s
}

func itoa(n int) string { return fmt.Sprint(n) }
