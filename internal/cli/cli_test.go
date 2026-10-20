package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alhafoudh/sharryctl/sharry"
)

// run executes the CLI with args against the config dir and returns stdout.
func run(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	cmd := NewRootCmd()
	var out, errb bytes.Buffer
	cmd.SetArgs(args)
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetIn(strings.NewReader(stdin))
	err := cmd.Execute()
	return out.String(), err
}

func TestCLILoginAndChangePlate(t *testing.T) {
	plates := []sharry.LicensePlate{{UUID: "p1", LicensePlate: "BA123AB", Default: ptr(true)}}
	var patchBody string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		full := "http://" + r.Host + r.URL.RequestURI()
		if r.Header.Get("Request-Sign") != sharry.Sign("sec", full, body) {
			t.Errorf("bad signature for %s %s", r.Method, full)
		}
		switch r.Method + " " + r.URL.Path {
		case "POST /api/v8/token":
			var req sharry.TokenRequest
			json.Unmarshal(body, &req)
			if req.GrantType == sharry.GrantCodeChallengeResponse {
				json.NewEncoder(w).Encode(map[string]any{
					"access_token": "acc", "refresh_token": "ref", "expires": time.Now().Add(time.Hour),
					"user": map[string]any{"id": 42, "uuid": "u-42", "fullname": "Test User", "email": req.Email},
				})
				return
			}
			io.WriteString(w, `{"user_status":"active","companies":[],"location_id":7}`)
		case "GET /api/v8/parking/license-plates":
			json.NewEncoder(w).Encode(map[string]any{"license_plates": plates})
		case "PATCH /api/v8/parking/license-plates/p1":
			patchBody = string(body)
			var req sharry.LicensePlateRequest
			json.Unmarshal(body, &req)
			plates[0].LicensePlate = req.LicensePlate
			json.NewEncoder(w).Encode(map[string]any{"updated": true, "license_plate": plates[0]})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("SHARRY_CONFIG_DIR", dir)
	t.Setenv("SHARRY_TENANT", "")
	cfg := "default_tenant = \"acme\"\n[tenants.acme]\napplication_token = \"app\"\napi_secret = \"sec\"\nbase_url = \"" + srv.URL + "/api/v8/\"\n"
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := run(t, "", "login", "--email", "Test@Example.com", "--code", "111111"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "profiles", "acme", "test@example.com.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("token file mode = %v, want 0600", info.Mode().Perm())
	}

	out, err := run(t, "", "plates", "change", "ba 123 ab", "--to", "ba-456-cd")
	if err != nil {
		t.Fatal(err)
	}
	if patchBody != `{"license_plate":"BA456CD","default":true}` {
		t.Fatalf("PATCH body = %s", patchBody)
	}
	if !strings.Contains(out, "Changed BA456CD (default) [p1]") {
		t.Fatalf("output = %q", out)
	}

	out, err = run(t, "", "--json", "plates", "list")
	if err != nil {
		t.Fatal(err)
	}
	var got []sharry.LicensePlate
	if err := json.Unmarshal([]byte(out), &got); err != nil || got[0].LicensePlate != "BA456CD" {
		t.Fatalf("list json = %s (%v)", out, err)
	}
}

func ptr[T any](v T) *T { return &v }
