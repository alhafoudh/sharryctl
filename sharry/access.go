package sharry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// Badge, card-format, device and verification payloads are not fully specified
// yet, so these helpers pass JSON through untouched. Typed models will replace
// json.RawMessage as the contracts are confirmed against a live tenant.

// ListBadges returns the user's access badges.
func (c *Client) ListBadges(ctx context.Context, user string) (json.RawMessage, error) {
	var out json.RawMessage
	err := c.Do(ctx, http.MethodGet, "access/users/"+url.PathEscape(user)+"/badges", nil, nil, &out, true)
	return out, err
}

// GrantBadge issues a badge; body is the issue-card request.
func (c *Client) GrantBadge(ctx context.Context, user string, body json.RawMessage) (json.RawMessage, error) {
	var out json.RawMessage
	err := c.Do(ctx, http.MethodPost, "access/users/"+url.PathEscape(user)+"/badges", nil, body, &out, true)
	return out, err
}

// RevokeBadge deletes a badge.
func (c *Client) RevokeBadge(ctx context.Context, user, badge string) (json.RawMessage, error) {
	var out json.RawMessage
	err := c.Do(ctx, http.MethodDelete, "access/users/"+url.PathEscape(user)+"/badges/"+url.PathEscape(badge), nil, nil, &out, true)
	return out, err
}

// SetBadgeState changes a badge's state.
func (c *Client) SetBadgeState(ctx context.Context, user, badge, state string) (json.RawMessage, error) {
	var out json.RawMessage
	path := "access/users/" + url.PathEscape(user) + "/badges/" + url.PathEscape(badge) + "/" + url.PathEscape(state)
	err := c.Do(ctx, http.MethodPatch, path, nil, nil, &out, true)
	return out, err
}

// ListCardFormats returns badge formats, optionally filtered by site IDs and type.
func (c *Client) ListCardFormats(ctx context.Context, user string, siteIDs []string, typ string) (json.RawMessage, error) {
	q := url.Values{}
	for _, id := range siteIDs {
		q.Add("filter[site_ids][]", id)
	}
	if typ != "" {
		q.Set("filter[type]", typ)
	}
	var out json.RawMessage
	err := c.Do(ctx, http.MethodGet, "users/"+url.PathEscape(user)+"/badge-formats", q, nil, &out, true)
	return out, err
}

// SetDeviceState changes a device's state; body is the change-device-state request.
func (c *Client) SetDeviceState(ctx context.Context, user, device string, body json.RawMessage) (json.RawMessage, error) {
	var out json.RawMessage
	err := c.Do(ctx, http.MethodPut, "users/"+url.PathEscape(user)+"/devices/"+url.PathEscape(device), nil, body, &out, true)
	return out, err
}

// SubmitVerification sends {"verification": verification}.
func (c *Client) SubmitVerification(ctx context.Context, user string, verification json.RawMessage) (json.RawMessage, error) {
	var out json.RawMessage
	body := map[string]json.RawMessage{"verification": verification}
	err := c.Do(ctx, http.MethodPost, "users/"+url.PathEscape(user)+"/verification", nil, body, &out, true)
	return out, err
}
