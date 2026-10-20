package sharry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"
)

// ParkingSpot is a single parking spot.
type ParkingSpot struct {
	UUID         string          `json:"uuid"`
	CompanyUUID  string          `json:"company_uuid"`
	LocationUUID string          `json:"location_uuid"`
	Number       any             `json:"number"`
	Occupied     *bool           `json:"occupied"`
	IsPublic     *bool           `json:"is_public"`
	Status       string          `json:"status"`
	Label        string          `json:"label"`
	Type         string          `json:"type"`
	LicensePlate *string         `json:"license_plate"`
	Location     json.RawMessage `json:"location,omitempty"`
}

// ParkingReservation is a booked spot for a time window.
type ParkingReservation struct {
	UUID            string       `json:"uuid"`
	ArrivalUUID     *string      `json:"arrival_uuid"`
	CompanyUUID     string       `json:"company_uuid"`
	DeletedAt       *string      `json:"deleted_at"`
	EmployeeUUID    *string      `json:"employee_uuid"`
	Start           string       `json:"start"`
	End             string       `json:"end"`
	GuestState      any          `json:"guest_state"`
	IsVisitor       bool         `json:"is_visitor"`
	LicensePlate    *string      `json:"license_plate"`
	ReservationID   any          `json:"reservation_id"`
	ReservationType string       `json:"reservation_type"`
	Spot            *ParkingSpot `json:"spot"`
	SpotNumber      any          `json:"spot_number"`
	SpotUUID        string       `json:"spot_uuid"`
	UserUUID        string       `json:"user_uuid"`
	VisitorUUID     *string      `json:"visitor_uuid"`
}

// SpotCounts summarizes spot availability.
type SpotCounts struct {
	Total          int `json:"total"`
	Free           int `json:"free"`
	Occupied       int `json:"occupied"`
	Reserved       int `json:"reserved"`
	PoolFree       int `json:"pool_free"`
	PublicFree     int `json:"public_free"`
	PublicOccupied int `json:"public_occupied"`
	PublicReserved int `json:"public_reserved"`
}

// ParkingCompany is a company's parking allocation.
type ParkingCompany struct {
	UUID            string     `json:"uuid"`
	Name            string     `json:"name"`
	GuaranteedSpots int        `json:"guaranteed_spots"`
	AcceptVisitors  bool       `json:"accept_visitors"`
	SpotsCount      SpotCounts `json:"spots_count"`
	SpotsAllowed    struct {
		Bookable int `json:"bookable"`
		Pool     int `json:"pool"`
		Visitor  int `json:"visitor"`
	} `json:"spots_allowed"`
}

// ParkingStatus is the availability for a company and the whole site.
type ParkingStatus struct {
	Companies  []ParkingCompany `json:"companies"`
	SpotsCount SpotCounts       `json:"spots_count"`
}

// ParkingStatus returns spot counts for the company and the site.
func (c *Client) ParkingStatus(ctx context.Context, companyUUID string) (*ParkingStatus, error) {
	var out ParkingStatus
	err := c.Do(ctx, http.MethodGet, "parking/companies/"+url.PathEscape(companyUUID)+"/spots", nil, nil, &out, true)
	return &out, err
}

// ParkingRecommendations returns spots that can be booked for the window.
func (c *Client) ParkingRecommendations(ctx context.Context, companyUUID string, from, to time.Time) ([]ParkingSpot, error) {
	q := url.Values{"from": {from.Format(time.RFC3339)}, "to": {to.Format(time.RFC3339)}}
	var out []ParkingSpot
	err := c.Do(ctx, http.MethodGet, "parking/companies/"+url.PathEscape(companyUUID)+"/recommendations", q, nil, &out, true)
	return out, err
}

// ListParkingReservations returns the user's reservations.
func (c *Client) ListParkingReservations(ctx context.Context, userUUID string) ([]ParkingReservation, error) {
	var out []ParkingReservation
	err := c.Do(ctx, http.MethodGet, "parking/users/"+url.PathEscape(userUUID)+"/reservations", nil, nil, &out, true)
	return out, err
}

// CreateParkingReservation books spotUUID for the window.
func (c *Client) CreateParkingReservation(ctx context.Context, userUUID, spotUUID string, from, to time.Time) (*ParkingReservation, error) {
	body := map[string]string{"spot_uuid": spotUUID, "from": from.Format(time.RFC3339), "to": to.Format(time.RFC3339)}
	var out ParkingReservation
	err := c.Do(ctx, http.MethodPost, "parking/users/"+url.PathEscape(userUUID)+"/reservations", nil, body, &out, true)
	return &out, err
}

// CancelParkingReservation deletes a reservation.
func (c *Client) CancelParkingReservation(ctx context.Context, userUUID, reservationUUID string) error {
	return c.Do(ctx, http.MethodDelete,
		"parking/users/"+url.PathEscape(userUUID)+"/reservations/"+url.PathEscape(reservationUUID), nil, nil, nil, true)
}

// ReleaseParkingReservation frees the spot for part of a reservation (e.g. leaving early).
func (c *Client) ReleaseParkingReservation(ctx context.Context, userUUID, reservationUUID string, from, to time.Time) (*ParkingReservation, error) {
	body := map[string]string{"release_from": from.Format(time.RFC3339), "release_to": to.Format(time.RFC3339)}
	var out ParkingReservation
	err := c.Do(ctx, http.MethodPost,
		"parking/users/"+url.PathEscape(userUUID)+"/reservations/"+url.PathEscape(reservationUUID)+"/release", nil, body, &out, true)
	return &out, err
}
