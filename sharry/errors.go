package sharry

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// APIError is returned for non-2xx responses.
type APIError struct {
	StatusCode int
	Message    string
	Body       []byte
	// RateLimitRemaining is -1 when the header was absent.
	RateLimitRemaining int
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("sharry: HTTP %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("sharry: HTTP %d", e.StatusCode)
}

// IsUnauthorized reports whether the error is a 401.
func (e *APIError) IsUnauthorized() bool { return e.StatusCode == http.StatusUnauthorized }

func newAPIError(resp *http.Response, body []byte) *APIError {
	e := &APIError{StatusCode: resp.StatusCode, Body: body, RateLimitRemaining: -1}
	if v := resp.Header.Get("X-Ratelimit-Remaining"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			e.RateLimitRemaining = n
		}
	}
	var payload struct {
		Message string `json:"message"`
		Error   any    `json:"error"`
		Errors  any    `json:"errors"`
	}
	if json.Unmarshal(body, &payload) == nil {
		switch {
		case payload.Message != "":
			e.Message = payload.Message
		case payload.Error != nil:
			e.Message = fmt.Sprint(payload.Error)
		case payload.Errors != nil:
			e.Message = fmt.Sprint(payload.Errors)
		}
	}
	if e.Message == "" && len(body) > 0 && len(body) < 512 {
		e.Message = string(body)
	}
	return e
}

// retryAfter picks a backoff for a 429 from Retry-After, falling back to exponential delay.
func retryAfter(resp *http.Response, attempt int) time.Duration {
	if v := resp.Header.Get("Retry-After"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs >= 0 && secs <= 120 {
			return time.Duration(secs) * time.Second
		}
	}
	return time.Duration(1<<attempt) * 2 * time.Second
}
