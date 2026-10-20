package sharry

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const (
	testAppToken = "app-token"
	testSecret   = "api-secret"
)

// fakeServer verifies signing on every request and records them.
type fakeServer struct {
	t        *testing.T
	srv      *httptest.Server
	handlers map[string]http.HandlerFunc // key: "METHOD /path"
	calls    []string
}

func newFakeServer(t *testing.T) *fakeServer {
	f := &fakeServer{t: t, handlers: map[string]http.HandlerFunc{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeServer) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	fullURL := "http://" + r.Host + r.URL.RequestURI()
	if got, want := r.Header.Get("Request-Sign"), Sign(testSecret, fullURL, body); got != want {
		f.t.Errorf("%s %s: bad Request-Sign %q, want %q", r.Method, fullURL, got, want)
	}
	if r.Header.Get("Application-Token") != testAppToken {
		f.t.Errorf("missing Application-Token")
	}
	key := r.Method + " " + strings.TrimPrefix(r.URL.Path, "/api/v8")
	f.calls = append(f.calls, key)
	h, ok := f.handlers[key]
	if !ok {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	r.Body = io.NopCloser(strings.NewReader(string(body)))
	h(w, r)
}

func (f *fakeServer) client(t *testing.T) *Client {
	c, err := NewClient(Config{
		Tenant: "example", ApplicationToken: testAppToken, APISecret: testSecret,
		BaseURL: f.srv.URL + "/api/v8/",
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestLoginFlowAndPlates(t *testing.T) {
	f := newFakeServer(t)
	expires := time.Now().Add(30 * 24 * time.Hour).UTC().Truncate(time.Second)

	f.handlers["POST /token"] = func(w http.ResponseWriter, r *http.Request) {
		var req TokenRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.ClientID != testAppToken {
			t.Errorf("client_id = %q", req.ClientID)
		}
		switch req.GrantType {
		case GrantCheckUser, GrantCodeChallenge:
			io.WriteString(w, `{"user_status":"active","companies":[],"location_id":1}`)
		case GrantCodeChallengeResponse:
			if req.Code != "123456" {
				http.Error(w, `{"message":"bad code"}`, 401)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{
				"user_status": "active", "access_token": "acc1", "refresh_token": "ref1",
				"expires": expires, "user": map[string]any{"uuid": "u-1", "email": "a@b.c"},
			})
		}
	}
	f.handlers["GET /parking/license-plates"] = func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Access-Token") != "acc1" {
			t.Errorf("Access-Token = %q", r.Header.Get("Access-Token"))
		}
		io.WriteString(w, `{"license_plates":[{"uuid":"p1","license_plate":"BA123AB","default":true}]}`)
	}
	f.handlers["POST /parking/license-plates"] = func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if string(b) != `{"license_plate":"BL999XY","default":false}` {
			t.Errorf("add body = %s", b)
		}
		io.WriteString(w, `{"created":true,"license_plate":{"uuid":"p2","license_plate":"BL999XY","default":false}}`)
	}
	f.handlers["PATCH /parking/license-plates/p2"] = func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"updated":true,"license_plate":{"uuid":"p2","license_plate":"BL111XY","default":false}}`)
	}
	f.handlers["DELETE /parking/license-plates/p2"] = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}

	c := f.client(t)
	ctx := context.Background()
	var saved Token
	c.SetSession(NewSession(Token{}, func(tk Token) { saved = tk }))

	if _, err := c.CheckUser(ctx, "a@b.c"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.RequestCode(ctx, "a@b.c", CodeChallengeOptions{}); err != nil {
		t.Fatal(err)
	}
	login, err := c.SubmitCode(ctx, "a@b.c", "123456")
	if err != nil {
		t.Fatal(err)
	}
	if login.User.UUID != "u-1" || saved.AccessToken != "acc1" || saved.RefreshToken != "ref1" {
		t.Fatalf("login not persisted: %+v %+v", login, saved)
	}

	plates, err := c.ListLicensePlates(ctx)
	if err != nil || len(plates) != 1 || plates[0].LicensePlate != "BA123AB" {
		t.Fatalf("list = %+v, %v", plates, err)
	}
	no := false
	p, err := c.AddLicensePlate(ctx, LicensePlateRequest{LicensePlate: "BL999XY", Default: &no})
	if err != nil || p.UUID != "p2" {
		t.Fatalf("add = %+v, %v", p, err)
	}
	p, err = c.UpdateLicensePlate(ctx, "p2", LicensePlateRequest{LicensePlate: "BL111XY"})
	if err != nil || p.LicensePlate != "BL111XY" {
		t.Fatalf("update = %+v, %v", p, err)
	}
	if err := c.DeleteLicensePlate(ctx, "p2"); err != nil {
		t.Fatal(err)
	}
}

func TestAutoRefresh(t *testing.T) {
	f := newFakeServer(t)
	f.handlers["GET /external-auths/response"] = func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("uuid") != "ref-old" {
			t.Errorf("uuid = %q", r.URL.Query().Get("uuid"))
		}
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "acc-new", "refresh_token": "ref-new", "expires": time.Now().Add(time.Hour),
		})
	}
	f.handlers["GET /parking/license-plates"] = func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Access-Token") != "acc-new" {
			t.Errorf("expected refreshed token, got %q", r.Header.Get("Access-Token"))
		}
		io.WriteString(w, `{"license_plates":[]}`)
	}
	c := f.client(t)
	var saved Token
	c.SetSession(NewSession(Token{AccessToken: "acc-old", RefreshToken: "ref-old", Expires: time.Now().Add(-time.Hour)},
		func(tk Token) { saved = tk }))

	if _, err := c.ListLicensePlates(context.Background()); err != nil {
		t.Fatal(err)
	}
	if saved.AccessToken != "acc-new" || saved.RefreshToken != "ref-new" {
		t.Fatalf("refresh not persisted: %+v", saved)
	}
}

func TestAPIError(t *testing.T) {
	f := newFakeServer(t)
	c := f.client(t)
	c.SetSession(NewSession(Token{AccessToken: "x", Expires: time.Now().Add(time.Hour)}, nil))
	_, err := c.ListLicensePlates(context.Background())
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.StatusCode != 404 || apiErr.Message != "not found" {
		t.Fatalf("err = %#v", err)
	}
}

func TestNewClientRequiresTenantData(t *testing.T) {
	if _, err := NewClient(Config{ApplicationToken: "a", APISecret: "b"}); err == nil {
		t.Fatal("expected error without tenant")
	}
	c, err := NewClient(Config{Tenant: "acme", ApplicationToken: "a", APISecret: "b"})
	if err != nil {
		t.Fatal(err)
	}
	if got := c.URL("token", nil); got != "https://acme.sharryapp.com/api/v8/token" {
		t.Fatalf("URL = %s", got)
	}
}
