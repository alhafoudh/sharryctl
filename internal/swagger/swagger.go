// Package swagger serves the bundled OpenAPI spec with Swagger UI and a local
// proxy that signs requests exactly like the rest of sharryctl.
package swagger

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/alhafoudh/sharryctl/sharry"
)

//go:embed ui
var uiFS embed.FS

// Load parses and validates an OpenAPI document.
func Load(ctx context.Context, data []byte) (*openapi3.T, error) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(data)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if err := doc.Validate(ctx); err != nil {
		return nil, fmt.Errorf("validate: %w", err)
	}
	return doc, nil
}

// Handler serves Swagger UI at /, the spec at /openapi.json and a signing proxy at /proxy/.
// client signs requests; when authed is true the session's Access-Token is attached.
func Handler(doc *openapi3.T, client *sharry.Client, authed bool, label string) (http.Handler, error) {
	spec := *doc
	spec.Servers = openapi3.Servers{{URL: "/proxy", Description: "sharryctl signing proxy -> " + label}}
	specJSON, err := json.Marshal(&spec)
	if err != nil {
		return nil, err
	}
	ui, err := fs.Sub(uiFS, "ui")
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.Handle("/ui/", http.StripPrefix("/ui/", http.FileServer(http.FS(ui))))
	mux.HandleFunc("/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(specJSON)
	})
	mux.HandleFunc("/proxy/", func(w http.ResponseWriter, r *http.Request) {
		proxy(w, r, client, authed)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, indexHTML)
	})
	return localOnly(mux), nil
}

func proxy(w http.ResponseWriter, r *http.Request, client *sharry.Client, authed bool) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	target := client.URL(strings.TrimPrefix(r.URL.Path, "/proxy/"), nil)
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	var token string
	if authed {
		if token, err = client.AccessToken(r.Context()); err != nil {
			http.Error(w, "sharryctl: "+err.Error(), http.StatusUnauthorized)
			return
		}
	}
	resp, err := client.Raw(r.Context(), r.Method, target, body, token)
	if err != nil {
		http.Error(w, "sharryctl: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// localOnly rejects requests whose Host is not loopback, blocking DNS-rebinding
// attacks against the proxy (which holds the user's credentials).
func localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if i := strings.LastIndex(host, ":"); i >= 0 && !strings.HasSuffix(host, "]") {
			host = host[:i]
		}
		switch host {
		case "localhost", "127.0.0.1", "[::1]":
			next.ServeHTTP(w, r)
		default:
			http.Error(w, "forbidden host", http.StatusForbidden)
		}
	})
}

const indexHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>sharryctl - Sharry API</title>
  <link rel="stylesheet" href="/ui/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="/ui/swagger-ui-bundle.js"></script>
  <script>
    window.ui = SwaggerUIBundle({
      url: "/openapi.json",
      dom_id: "#swagger-ui",
      deepLinking: true,
      tryItOutEnabled: true,
      persistAuthorization: false
    });
  </script>
</body>
</html>
`
