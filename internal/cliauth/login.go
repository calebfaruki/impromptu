package cliauth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"time"
)

// Login performs browser-based OAuth login for the CLI.
// Opens the user's browser to the registry login page, captures the session
// token from the localhost callback, and returns it.
func Login(ctx context.Context, registryURL string) (string, error) {
	tokenCh := make(chan string, 1)
	errCh := make(chan error, 1)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("starting localhost server: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token == "" {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, "Missing token. Please try again.")
			errCh <- fmt.Errorf("callback received without token")
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html><body><h1>Login successful</h1><p>You can close this tab and return to the terminal.</p></body></html>")
		tokenCh <- token
	})

	srv := &http.Server{Handler: mux}
	go func() {
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	callbackURL := fmt.Sprintf("http://localhost:%d/callback", port)
	loginURL := fmt.Sprintf("%s/login?cli_redirect=%s", registryURL, callbackURL)

	if err := openBrowser(loginURL); err != nil {
		srv.Close()
		return "", fmt.Errorf("opening browser: %w (manual URL: %s)", err, loginURL)
	}

	select {
	case token := <-tokenCh:
		srv.Close()
		return token, nil
	case err := <-errCh:
		srv.Close()
		return "", err
	case <-time.After(5 * time.Minute):
		srv.Close()
		return "", fmt.Errorf("login timed out after 5 minutes")
	case <-ctx.Done():
		srv.Close()
		return "", ctx.Err()
	}
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "windows":
		return exec.Command("cmd", "/c", "start", url).Start()
	default:
		return fmt.Errorf("unsupported platform %s", runtime.GOOS)
	}
}
