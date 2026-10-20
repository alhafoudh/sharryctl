package cli

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/alhafoudh/sharryctl/api"
	"github.com/alhafoudh/sharryctl/internal/swagger"
	"github.com/alhafoudh/sharryctl/sharry"
)

func (a *app) swaggerCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "swagger", Short: "Explore the bundled OpenAPI spec"}

	var port int
	var open, noAuth bool
	serve := &cobra.Command{
		Use:   "serve",
		Short: "Serve Swagger UI with a local signing proxy on 127.0.0.1",
		Long: `Serves Swagger UI for the bundled spec. "Try it out" requests go to a local
proxy that signs them and adds the logged-in profile's Access-Token, so you
never paste secrets into the browser. The server only listens on 127.0.0.1.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			doc, err := swagger.Load(ctx(cmd), api.OpenAPI)
			if err != nil {
				return err
			}
			var client *sharry.Client
			authed := false
			label := ""
			if !noAuth {
				c, pr, err := a.authedClient()
				if err == nil {
					client, authed, label = c, true, pr.Tenant+" as "+pr.Email
				} else {
					fmt.Fprintln(a.err, "note:", err, "- proxy will send unauthenticated requests")
				}
			}
			if client == nil {
				c, t, err := a.newClient()
				if err != nil {
					return err
				}
				client, label = c, t.Slug
			}
			h, err := swagger.Handler(doc, client, authed, label)
			if err != nil {
				return err
			}
			ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
			if err != nil {
				return err
			}
			url := fmt.Sprintf("http://localhost:%d/", ln.Addr().(*net.TCPAddr).Port)
			fmt.Fprintf(a.err, "Swagger UI for %s at %s (Ctrl-C to stop)\n", label, url)
			if open {
				openBrowser(url)
			}
			srv := &http.Server{Handler: h}
			go func() { <-ctx(cmd).Done(); srv.Close() }()
			if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			return nil
		},
	}
	serve.Flags().IntVar(&port, "port", 8088, "listen port (0 for random)")
	serve.Flags().BoolVar(&open, "open", false, "open the browser")
	serve.Flags().BoolVar(&noAuth, "no-auth", false, "do not attach the profile's Access-Token")

	validate := &cobra.Command{
		Use:   "validate [file]",
		Short: "Validate the bundled spec, or an OpenAPI file",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, name := api.OpenAPI, "bundled spec"
			if len(args) == 1 {
				var err error
				if data, err = os.ReadFile(args[0]); err != nil {
					return err
				}
				name = args[0]
			}
			doc, err := swagger.Load(ctx(cmd), data)
			if err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			n := 0
			for _, p := range doc.Paths.Map() {
				n += len(p.Operations())
			}
			if a.g.json {
				return a.printJSON(map[string]any{"valid": true, "file": name, "operations": n})
			}
			fmt.Fprintf(a.out, "%s is valid (%d operations).\n", name, n)
			return nil
		},
	}
	cmd.AddCommand(serve, validate, &cobra.Command{
		Use:   "spec",
		Short: "Print the bundled OpenAPI spec (YAML)",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := a.out.Write(api.OpenAPI)
			return err
		},
	})
	return cmd
}

func openBrowser(url string) {
	var c *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		c = exec.Command("open", url)
	case "windows":
		c = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		c = exec.Command("xdg-open", url)
	}
	_ = c.Start()
}
