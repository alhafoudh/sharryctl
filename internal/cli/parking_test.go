package cli

import (
	"testing"
	"time"
)

func TestParseWhen(t *testing.T) {
	ref := time.Date(2026, 10, 21, 9, 0, 0, 0, time.Local)
	tomorrow := time.Now().AddDate(0, 0, 1)
	tests := []struct {
		in   string
		want time.Time
	}{
		{"17:30", time.Date(2026, 10, 21, 17, 30, 0, 0, time.Local)},
		{"2026-10-22 08:15", time.Date(2026, 10, 22, 8, 15, 0, 0, time.Local)},
		{"2026-10-22T08:15:00+02:00", time.Date(2026, 10, 22, 8, 15, 0, 0, time.FixedZone("", 2*3600))},
		{"tomorrow 07:00", time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 7, 0, 0, 0, time.Local)},
	}
	for _, tt := range tests {
		got, err := parseWhen(tt.in, ref)
		if err != nil {
			t.Fatalf("%q: %v", tt.in, err)
		}
		if !got.Equal(tt.want) {
			t.Errorf("%q = %v, want %v", tt.in, got, tt.want)
		}
	}
	if _, err := parseWhen("next blursday", ref); err == nil {
		t.Error("expected error for garbage")
	}
}

func TestParseWindowToFollowsFromDay(t *testing.T) {
	f, to, err := parseWindow("tomorrow 09:00", "17:00")
	if err != nil {
		t.Fatal(err)
	}
	if to.Sub(f) != 8*time.Hour {
		t.Fatalf("window = %v - %v", f, to)
	}
	if _, _, err := parseWindow("tomorrow 17:00", "09:00"); err == nil {
		t.Fatal("expected error when --to is before --from")
	}
}
