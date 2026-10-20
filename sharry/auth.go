package sharry

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// Grant types accepted by POST token.
const (
	GrantCheckUser             = "check_user"
	GrantCodeChallenge         = "code_challenge"
	GrantCodeChallengeResend   = "code_challenge_resend"
	GrantCodeChallengeResponse = "code_challenge_response"
)

// TokenRequest is the body of POST token. Optional fields are omitted when empty.
type TokenRequest struct {
	Email            string `json:"email"`
	ClientID         string `json:"client_id"`
	GrantType        string `json:"grant_type"`
	Code             string `json:"code,omitempty"`
	LocationID       any    `json:"location_id,omitempty"`
	CompanyID        any    `json:"company_id,omitempty"`
	AgreeWithPrivacy *bool  `json:"agree_with_privacy,omitempty"`
}

// IdentityResponse is returned by check_user and code_challenge.
type IdentityResponse struct {
	UserStatus  string          `json:"user_status"`
	Companies   json.RawMessage `json:"companies"`
	LocationID  any             `json:"location_id"`
	SiteID      any             `json:"site_id,omitempty"`
	SSOLink     *string         `json:"sso_link"`
	SSOProvider *string         `json:"sso_provider"`
}

// LoginResponse is returned by code_challenge_response.
type LoginResponse struct {
	UserStatus   string    `json:"user_status"`
	User         User      `json:"user"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	Expires      time.Time `json:"expires"`
	UserID       any       `json:"user_id"`
	CompanyID    any       `json:"company_id"`
	CompanyUUID  string    `json:"company_uuid"`
}

// RefreshResponse is returned by GET external-auths/response.
type RefreshResponse struct {
	AccessToken  string    `json:"access_token"`
	Expires      time.Time `json:"expires"`
	RefreshToken string    `json:"refresh_token"`
}

// CheckUser runs the identity check for email.
func (c *Client) CheckUser(ctx context.Context, email string) (*IdentityResponse, error) {
	var out IdentityResponse
	err := c.Do(ctx, http.MethodPost, "token", nil, TokenRequest{
		Email: email, ClientID: c.cfg.ApplicationToken, GrantType: GrantCheckUser,
	}, &out, false)
	return &out, err
}

// CodeChallengeOptions are the optional fields of a code_challenge request.
type CodeChallengeOptions struct {
	LocationID       any
	CompanyID        any
	AgreeWithPrivacy *bool
}

// RequestCode asks the server to email a login code.
func (c *Client) RequestCode(ctx context.Context, email string, opts CodeChallengeOptions) (*IdentityResponse, error) {
	var out IdentityResponse
	err := c.Do(ctx, http.MethodPost, "token", nil, TokenRequest{
		Email: email, ClientID: c.cfg.ApplicationToken, GrantType: GrantCodeChallenge,
		LocationID: opts.LocationID, CompanyID: opts.CompanyID, AgreeWithPrivacy: opts.AgreeWithPrivacy,
	}, &out, false)
	return &out, err
}

// ResendCode re-sends the login code.
func (c *Client) ResendCode(ctx context.Context, email string) error {
	return c.Do(ctx, http.MethodPost, "token", nil, TokenRequest{
		Email: email, ClientID: c.cfg.ApplicationToken, GrantType: GrantCodeChallengeResend,
	}, nil, false)
}

// SubmitCode exchanges the emailed code for tokens and attaches the resulting session.
func (c *Client) SubmitCode(ctx context.Context, email, code string) (*LoginResponse, error) {
	var out LoginResponse
	if err := c.Do(ctx, http.MethodPost, "token", nil, TokenRequest{
		Email: email, ClientID: c.cfg.ApplicationToken, GrantType: GrantCodeChallengeResponse, Code: code,
	}, &out, false); err != nil {
		return nil, err
	}
	if c.session == nil {
		c.session = &Session{}
	}
	c.session.update(Token{AccessToken: out.AccessToken, RefreshToken: out.RefreshToken, Expires: out.Expires})
	return &out, nil
}

// Refresh exchanges the session's refresh token for a new access token.
func (c *Client) Refresh(ctx context.Context) (*RefreshResponse, error) {
	if c.session == nil || c.session.Token().RefreshToken == "" {
		return nil, ErrNotLoggedIn
	}
	var out RefreshResponse
	q := url.Values{"uuid": {c.session.Token().RefreshToken}}
	if err := c.Do(ctx, http.MethodGet, "external-auths/response", q, nil, &out, false); err != nil {
		return nil, err
	}
	if out.AccessToken == "" {
		return nil, errors.New("sharry: refresh returned no access_token")
	}
	t := c.session.Token()
	t.AccessToken, t.Expires = out.AccessToken, out.Expires
	if out.RefreshToken != "" {
		t.RefreshToken = out.RefreshToken
	}
	c.session.update(t)
	return &out, nil
}

// ErrNotLoggedIn is returned when an authenticated call has no session.
var ErrNotLoggedIn = errors.New("sharry: not logged in")

// Token is the user credential pair.
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	Expires      time.Time `json:"expires"`
}

// Expired reports whether the access token expires within skew.
func (t Token) Expired(skew time.Duration) bool {
	return t.AccessToken == "" || (!t.Expires.IsZero() && time.Now().Add(skew).After(t.Expires))
}

// Session holds user tokens and notifies OnUpdate when they change (e.g. to persist them).
type Session struct {
	mu       sync.Mutex
	token    Token
	OnUpdate func(Token)
}

// NewSession creates a session from stored tokens.
func NewSession(t Token, onUpdate func(Token)) *Session {
	return &Session{token: t, OnUpdate: onUpdate}
}

// Token returns a copy of the current token.
func (s *Session) Token() Token {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.token
}

func (s *Session) update(t Token) {
	s.mu.Lock()
	s.token = t
	cb := s.OnUpdate
	s.mu.Unlock()
	if cb != nil {
		cb(t)
	}
}

// accessToken returns a valid access token, refreshing when it is expired or about to.
func (c *Client) accessToken(ctx context.Context) (string, error) {
	if c.session == nil {
		return "", ErrNotLoggedIn
	}
	t := c.session.Token()
	if t.Expired(5 * time.Minute) {
		if t.RefreshToken == "" {
			return "", ErrNotLoggedIn
		}
		if _, err := c.Refresh(ctx); err != nil {
			return "", err
		}
		t = c.session.Token()
	}
	return t.AccessToken, nil
}

// AccessToken returns a valid (refreshed if needed) access token for external use.
func (c *Client) AccessToken(ctx context.Context) (string, error) { return c.accessToken(ctx) }
