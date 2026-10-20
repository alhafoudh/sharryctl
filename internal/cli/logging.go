package cli

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"
)

type loggingTransport struct {
	next http.RoundTripper
	w    io.Writer
}

var redactQuery = regexp.MustCompile(`(uuid=)[^&]+`)

func newLoggingHTTPClient(w io.Writer) *http.Client {
	return &http.Client{Timeout: 30 * time.Second, Transport: &loggingTransport{next: http.DefaultTransport, w: w}}
}

func (l *loggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	url := redactQuery.ReplaceAllString(req.URL.String(), "${1}<redacted>")
	resp, err := l.next.RoundTrip(req)
	if err != nil {
		fmt.Fprintf(l.w, "> %s %s: %v\n", req.Method, url, err)
		return nil, err
	}
	fmt.Fprintf(l.w, "> %s %s -> %d (%s) ratelimit=%s/%s\n", req.Method, url, resp.StatusCode,
		time.Since(start).Round(time.Millisecond),
		resp.Header.Get("X-Ratelimit-Remaining"), resp.Header.Get("X-Ratelimit-Limit"))
	return resp, nil
}
