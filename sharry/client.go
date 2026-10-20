// Package sharry is a Go SDK for the Sharry workplace platform API.
//
// Sharry is multi-tenant: every tenant is a separate deployment reachable at
// https://<tenant>.sharryapp.com/api/v<version>/. Nothing in this package is
// tied to a specific tenant; all tenant data comes from Config.
package sharry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultAPIVersion = "v8"
	DefaultPlatform   = "Android"
	DefaultLanguage   = "en"
	DefaultAppVersion = "4.15.0"
	DefaultBuild      = "1"
)

// Config holds per-tenant settings. Tenant, ApplicationToken and APISecret are required.
type Config struct {
	Tenant           string // tenant slug, e.g. "acme"
	APIVersion       string // defaults to DefaultAPIVersion
	ApplicationToken string
	APISecret        string

	// BaseURL overrides https://<tenant>.sharryapp.com/api/<version>/ (tests, proxies).
	BaseURL string

	Platform   string
	Language   string
	AppVersion string
	// UserAgent overrides the generated "<AppName>/<ver> (<package>; build:<n>; <platform>)".
	UserAgent string
	AppName   string // used in the generated User-Agent; defaults to "Sharry"
	Package   string // used in the generated User-Agent; defaults to "eu.sharry.swp"
	Build     string

	HTTPClient *http.Client
}

// Client calls the Sharry API with application-level signing and optional user auth.
type Client struct {
	cfg     Config
	base    *url.URL
	http    *http.Client
	session *Session
}

// NewClient validates cfg, fills defaults and returns a Client.
func NewClient(cfg Config) (*Client, error) {
	if cfg.ApplicationToken == "" || cfg.APISecret == "" {
		return nil, errors.New("sharry: ApplicationToken and APISecret are required")
	}
	if cfg.APIVersion == "" {
		cfg.APIVersion = DefaultAPIVersion
	}
	if cfg.BaseURL == "" {
		if cfg.Tenant == "" {
			return nil, errors.New("sharry: Tenant is required")
		}
		cfg.BaseURL = fmt.Sprintf("https://%s.sharryapp.com/api/%s/", cfg.Tenant, cfg.APIVersion)
	}
	if !strings.HasSuffix(cfg.BaseURL, "/") {
		cfg.BaseURL += "/"
	}
	base, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("sharry: invalid base URL: %w", err)
	}
	if cfg.Platform == "" {
		cfg.Platform = DefaultPlatform
	}
	if cfg.Language == "" {
		cfg.Language = DefaultLanguage
	}
	if cfg.AppVersion == "" {
		cfg.AppVersion = DefaultAppVersion
	}
	if cfg.AppName == "" {
		cfg.AppName = "Sharry"
	}
	if cfg.Package == "" {
		cfg.Package = "eu.sharry.swp"
	}
	if cfg.Build == "" {
		cfg.Build = DefaultBuild
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = fmt.Sprintf("%s/%s (%s; build:%s; %s)", cfg.AppName, cfg.AppVersion, cfg.Package, cfg.Build, cfg.Platform)
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{cfg: cfg, base: base, http: hc}, nil
}

// Config returns the effective configuration (with defaults applied).
func (c *Client) Config() Config { return c.cfg }

// URL resolves an API-relative path (e.g. "parking/license-plates") against the base URL.
func (c *Client) URL(path string, query url.Values) string {
	u := *c.base
	rel, _ := url.Parse(strings.TrimPrefix(path, "/"))
	r := u.ResolveReference(rel)
	if len(query) > 0 {
		r.RawQuery = query.Encode()
	}
	return r.String()
}

// SetSession attaches user credentials; authenticated calls use and refresh them.
func (c *Client) SetSession(s *Session) { c.session = s }

// Session returns the attached session, or nil.
func (c *Client) Session() *Session { return c.session }

// Do performs a signed request. body may be nil, []byte, json.RawMessage or any
// JSON-marshalable value. If out is non-nil the JSON response is decoded into it.
// When auth is true the request carries Access-Token, refreshing it if expired.
func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body, out any, auth bool) error {
	raw, err := encodeBody(body)
	if err != nil {
		return err
	}
	var accessToken string
	if auth {
		if accessToken, err = c.accessToken(ctx); err != nil {
			return err
		}
	}
	resp, err := c.Raw(ctx, method, c.URL(path, query), raw, accessToken)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return newAPIError(resp, data)
	}
	if out != nil && len(bytes.TrimSpace(data)) > 0 {
		if r, ok := out.(*json.RawMessage); ok {
			*r = append((*r)[:0], data...)
			return nil
		}
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("sharry: decode %s %s: %w", method, path, err)
		}
	}
	return nil
}

// Raw sends a fully signed request to fullURL and returns the unread response.
// It is the single place where signing and standard headers are applied, so
// higher-level helpers and the swagger proxy share one code path.
func (c *Client) Raw(ctx context.Context, method, fullURL string, body []byte, accessToken string) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, fullURL, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		c.applyHeaders(req, fullURL, body, accessToken)
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusTooManyRequests || attempt >= 2 {
			return resp, nil
		}
		wait := retryAfter(resp, attempt)
		resp.Body.Close()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
}

func (c *Client) applyHeaders(req *http.Request, fullURL string, body []byte, accessToken string) {
	h := req.Header
	h.Set("Application-Token", c.cfg.ApplicationToken)
	h.Set("Request-Sign", Sign(c.cfg.APISecret, fullURL, body))
	h.Set("Platform", c.cfg.Platform)
	h.Set("Language", c.cfg.Language)
	h.Set("App-Version", c.cfg.AppVersion)
	h.Set("Accept", "application/json")
	h.Set("Content-Type", "application/json; charset=UTF-8")
	h.Set("User-Agent", c.cfg.UserAgent)
	if accessToken != "" {
		h.Set("Access-Token", accessToken)
	}
}

func encodeBody(body any) ([]byte, error) {
	switch b := body.(type) {
	case nil:
		return nil, nil
	case []byte:
		return b, nil
	case json.RawMessage:
		return b, nil
	default:
		return json.Marshal(b)
	}
}
