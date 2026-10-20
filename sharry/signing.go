package sharry

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// Sign computes the Request-Sign header value for a request.
//
// signature = lowercase_hex(HMAC_SHA256(secret, fullURL + "|" + body))
//
// fullURL must be the exact URL sent on the wire, including the query string.
// body is the exact request body; pass nil or empty for requests without one.
func Sign(secret, fullURL string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(fullURL))
	mac.Write([]byte("|"))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
