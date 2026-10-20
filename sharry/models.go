package sharry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Image is a company image reference on the CDN.
type Image struct {
	UUID             string `json:"uuid"`
	URL              string `json:"url"`
	ThumbnailPattern string `json:"thumbnail_pattern"`
}

// Company is the user's company.
type Company struct {
	ID              any     `json:"id"`
	UUID            string  `json:"uuid"`
	ParentID        any     `json:"parent_id"`
	ParentUUID      *string `json:"parent_uuid"`
	Name            string  `json:"name"`
	Annotation      *string `json:"annotation"`
	Category        any     `json:"category"`
	Content         *string `json:"content"`
	ContentOriginal *string `json:"content_original"`
	Phone           *string `json:"phone"`
	URL             *string `json:"url"`
	Email           *string `json:"email"`
	Facebook        *string `json:"facebook"`
	LinkedIn        *string `json:"linkedin"`
	Images          []Image `json:"images"`
}

// Me returns the logged-in user's profile (GET users/me) and its raw JSON.
func (c *Client) Me(ctx context.Context) (*User, json.RawMessage, error) {
	var out struct {
		User json.RawMessage `json:"user"`
	}
	if err := c.Do(ctx, http.MethodGet, "users/me", nil, nil, &out, true); err != nil {
		return nil, nil, err
	}
	var u User
	if err := json.Unmarshal(out.User, &u); err != nil {
		return nil, nil, fmt.Errorf("sharry: decode users/me: %w", err)
	}
	return &u, out.User, nil
}

// User is the profile returned at login. Loosely-specified fields stay raw JSON.
type User struct {
	ID                         any             `json:"id"`
	UUID                       string          `json:"uuid"`
	Status                     string          `json:"status"`
	InvitationMailSentAt       *string         `json:"invitation_mail_sent_at"`
	AccessInvitationMailSentAt *string         `json:"access_invitation_mail_sent_at"`
	Company                    *Company        `json:"company"`
	CanEditEmail               bool            `json:"can_edit_email"`
	Anonymize                  bool            `json:"anonymize"`
	HasPhoto                   bool            `json:"has_photo"`
	Me                         bool            `json:"me"`
	Image                      json.RawMessage `json:"image"`
	Tags                       json.RawMessage `json:"tags"`
	VirtualCardRequestStatus   any             `json:"virtual_card_request_status"`
	UpdatingAccessRole         any             `json:"updating_access_role"`
	Email                      string          `json:"email"`
	Fullname                   string          `json:"fullname"`
	Phone                      *string         `json:"phone"`
	Name                       string          `json:"name"`
	Surname                    string          `json:"surname"`
	HasAnyAdminRole            bool            `json:"has_any_admin_role"`
	Permissions                json.RawMessage `json:"permissions"`
	Roles                      json.RawMessage `json:"roles"`
	Agreements                 json.RawMessage `json:"agreements"`
	Profession                 any             `json:"profession"`
	Building                   any             `json:"building"`
	Location                   any             `json:"location"`
}
