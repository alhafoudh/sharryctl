// Package config loads tenant configuration and stores per-profile tokens
// under the XDG config directory (~/.config/sharry by default).
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/alhafoudh/sharryctl/sharry"
)

// Tenant is one [tenants.<slug>] table in config.toml.
type Tenant struct {
	Slug             string `toml:"-"`
	APIVersion       string `toml:"api_version,omitempty"`
	ApplicationToken string `toml:"application_token"`
	APISecret        string `toml:"api_secret"`
	BaseURL          string `toml:"base_url,omitempty"`
	UserAgent        string `toml:"user_agent,omitempty"`
	AppName          string `toml:"app_name,omitempty"`
	Package          string `toml:"package,omitempty"`
	AppVersion       string `toml:"app_version,omitempty"`
	Build            string `toml:"build,omitempty"`
	Platform         string `toml:"platform,omitempty"`
	Language         string `toml:"language,omitempty"`
	DefaultProfile   string `toml:"default_profile,omitempty"`
}

// File is the whole config.toml.
type File struct {
	DefaultTenant string             `toml:"default_tenant,omitempty"`
	Tenants       map[string]*Tenant `toml:"tenants"`
}

// Dir returns the config directory, honoring SHARRY_CONFIG_DIR and XDG_CONFIG_HOME.
func Dir() (string, error) {
	if d := os.Getenv("SHARRY_CONFIG_DIR"); d != "" {
		return d, nil
	}
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "sharry"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "sharry"), nil
}

// Path returns the config.toml path.
func Path() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.toml"), nil
}

// Load reads config.toml; a missing file yields an empty config.
func Load() (*File, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	f := &File{Tenants: map[string]*Tenant{}}
	if _, err := toml.DecodeFile(p, f); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read %s: %w", p, err)
	}
	if f.Tenants == nil {
		f.Tenants = map[string]*Tenant{}
	}
	for slug, t := range f.Tenants {
		t.Slug = slug
	}
	return f, nil
}

// Save writes config.toml with 0600 permissions (it contains tenant secrets).
func (f *File) Save() error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	var b strings.Builder
	if err := toml.NewEncoder(&b).Encode(f); err != nil {
		return err
	}
	return writeFile0600(p, []byte(b.String()))
}

// Slugs returns tenant slugs sorted.
func (f *File) Slugs() []string {
	s := make([]string, 0, len(f.Tenants))
	for k := range f.Tenants {
		s = append(s, k)
	}
	sort.Strings(s)
	return s
}

// Resolve picks the tenant: explicit flag, SHARRY_TENANT, default_tenant, or the only one configured.
func (f *File) Resolve(flag string) (*Tenant, error) {
	slug := flag
	if slug == "" {
		slug = os.Getenv("SHARRY_TENANT")
	}
	if slug == "" {
		slug = f.DefaultTenant
	}
	if slug == "" && len(f.Tenants) == 1 {
		slug = f.Slugs()[0]
	}
	if slug == "" {
		return nil, errors.New("no tenant selected: pass --tenant, set SHARRY_TENANT, or run `sharryctl config tenants add`")
	}
	t, ok := f.Tenants[slug]
	if !ok {
		return nil, fmt.Errorf("tenant %q is not configured (see `sharryctl config tenants list`)", slug)
	}
	// Secrets may come from the environment instead of the file.
	if v := os.Getenv("SHARRY_APPLICATION_TOKEN"); v != "" {
		t.ApplicationToken = v
	}
	if v := os.Getenv("SHARRY_API_SECRET"); v != "" {
		t.APISecret = v
	}
	if t.ApplicationToken == "" || t.APISecret == "" {
		return nil, fmt.Errorf("tenant %q is missing application_token or api_secret", slug)
	}
	return t, nil
}

// SDKConfig converts a tenant to an SDK config.
func (t *Tenant) SDKConfig() sharry.Config {
	return sharry.Config{
		Tenant: t.Slug, APIVersion: t.APIVersion, ApplicationToken: t.ApplicationToken, APISecret: t.APISecret,
		BaseURL: t.BaseURL, UserAgent: t.UserAgent, AppName: t.AppName, Package: t.Package,
		AppVersion: t.AppVersion, Build: t.Build, Platform: t.Platform, Language: t.Language,
	}
}

// Profile is the stored login for tenant+email.
type Profile struct {
	Tenant  string          `json:"tenant"`
	Email   string          `json:"email"`
	Token   sharry.Token    `json:"token"`
	User    json.RawMessage `json:"user,omitempty"`
	Updated time.Time       `json:"updated"`
}

var unsafeChars = regexp.MustCompile(`[^A-Za-z0-9@._+-]`)

func profilePath(tenant, email string) (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	name := unsafeChars.ReplaceAllString(strings.ToLower(email), "_") + ".json"
	return filepath.Join(d, "profiles", unsafeChars.ReplaceAllString(tenant, "_"), name), nil
}

// LoadProfile reads a stored profile.
func LoadProfile(tenant, email string) (*Profile, error) {
	p, err := profilePath(tenant, email)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var pr Profile
	if err := json.Unmarshal(data, &pr); err != nil {
		return nil, fmt.Errorf("parse %s: %w", p, err)
	}
	return &pr, nil
}

// Save writes the profile with 0600 permissions.
func (pr *Profile) Save() error {
	p, err := profilePath(pr.Tenant, pr.Email)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	pr.Updated = time.Now().UTC()
	data, err := json.MarshalIndent(pr, "", "  ")
	if err != nil {
		return err
	}
	return writeFile0600(p, data)
}

// DeleteProfile removes a stored profile.
func DeleteProfile(tenant, email string) error {
	p, err := profilePath(tenant, email)
	if err != nil {
		return err
	}
	return os.Remove(p)
}

// ListProfiles returns stored emails for a tenant.
func ListProfiles(tenant string) ([]string, error) {
	d, err := Dir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(d, "profiles", unsafeChars.ReplaceAllString(tenant, "_")))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			if pr, err := LoadProfile(tenant, strings.TrimSuffix(e.Name(), ".json")); err == nil {
				out = append(out, pr.Email)
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

func writeFile0600(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
