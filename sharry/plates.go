package sharry

import (
	"context"
	"net/http"
	"net/url"
)

// LicensePlate is a vehicle plate registered for parking.
type LicensePlate struct {
	UUID         string `json:"uuid"`
	LicensePlate string `json:"license_plate"`
	Default      *bool  `json:"default"`
}

// LicensePlateRequest is the body for creating or changing a plate.
type LicensePlateRequest struct {
	LicensePlate string `json:"license_plate,omitempty"`
	Default      *bool  `json:"default,omitempty"`
}

// ListLicensePlates returns the current user's plates.
func (c *Client) ListLicensePlates(ctx context.Context) ([]LicensePlate, error) {
	var out struct {
		LicensePlates []LicensePlate `json:"license_plates"`
	}
	err := c.Do(ctx, http.MethodGet, "parking/license-plates", nil, nil, &out, true)
	return out.LicensePlates, err
}

// AddLicensePlate registers a new plate.
func (c *Client) AddLicensePlate(ctx context.Context, req LicensePlateRequest) (*LicensePlate, error) {
	var out struct {
		Created      *bool         `json:"created"`
		LicensePlate *LicensePlate `json:"license_plate"`
	}
	err := c.Do(ctx, http.MethodPost, "parking/license-plates", nil, req, &out, true)
	return out.LicensePlate, err
}

// UpdateLicensePlate changes a plate's number and/or default flag.
func (c *Client) UpdateLicensePlate(ctx context.Context, uuid string, req LicensePlateRequest) (*LicensePlate, error) {
	var out struct {
		Updated      *bool         `json:"updated"`
		LicensePlate *LicensePlate `json:"license_plate"`
	}
	err := c.Do(ctx, http.MethodPatch, "parking/license-plates/"+url.PathEscape(uuid), nil, req, &out, true)
	return out.LicensePlate, err
}

// DeleteLicensePlate removes a plate.
func (c *Client) DeleteLicensePlate(ctx context.Context, uuid string) error {
	return c.Do(ctx, http.MethodDelete, "parking/license-plates/"+url.PathEscape(uuid), nil, nil, nil, true)
}
