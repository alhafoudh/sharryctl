package sharry

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestSign(t *testing.T) {
	tests := []struct {
		name   string
		secret string
		url    string
		body   []byte
	}{
		{"post with body", "s3cret", "https://example.sharryapp.com/api/v8/token", []byte(`{"email":"a@b.c"}`)},
		{"get without body", "s3cret", "https://example.sharryapp.com/api/v8/external-auths/response?uuid=abc", nil},
		{"unicode secret", "tajné-heslo", "https://example.sharryapp.com/api/v8/token", []byte("{}")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mac := hmac.New(sha256.New, []byte(tt.secret))
			mac.Write([]byte(tt.url + "|" + string(tt.body)))
			want := hex.EncodeToString(mac.Sum(nil))

			if got := Sign(tt.secret, tt.url, tt.body); got != want {
				t.Fatalf("Sign() = %s, want %s", got, want)
			}
		})
	}
}

func TestSignKnownVector(t *testing.T) {
	// Computed independently with Ruby OpenSSL::HMAC so a change in message composition is caught.
	const want = "03588d5cb98404b0342d7e935de6de1bcded3a413d5151385539972975292591"
	if got := Sign("key", "https://example.sharryapp.com/api/v8/token", []byte("{}")); got != want {
		t.Fatalf("Sign() = %s, want %s", got, want)
	}
}
