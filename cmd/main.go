package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/calebfaruki/impromptu/internal/auth"
	"github.com/calebfaruki/impromptu/internal/contentcheck"
	"github.com/calebfaruki/impromptu/internal/index"
	"github.com/calebfaruki/impromptu/internal/registry"
	"github.com/calebfaruki/impromptu/web"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: impromptu <command> [args]\n")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "check":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "usage: impromptu check <directory>\n")
			os.Exit(1)
		}
		runCheck(os.Args[2])
	case "serve":
		runServe()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func runCheck(dir string) {
	violations, err := contentcheck.CheckDirectory(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}
	if len(violations) == 0 {
		fmt.Println("PASS")
		return
	}
	for _, v := range violations {
		fmt.Println(v.Error())
	}
	os.Exit(1)
}

func runServe() {
	addr := ":8080"
	if p := os.Getenv("PORT"); p != "" {
		addr = ":" + p
	}

	db, err := index.Open(":memory:")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	migrations := os.DirFS(".")
	if err := index.Migrate(context.Background(), db, migrations); err != nil {
		fmt.Fprintf(os.Stderr, "error running migrations: %v\n", err)
		os.Exit(1)
	}

	blobRoot := "./tmp/blobs"
	blobs, err := registry.NewFilesystemStore(blobRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating blob store: %v\n", err)
		os.Exit(1)
	}

	cookieKey := []byte("dev-mode-key-32-bytes-long-ok!!!")
	signer := auth.NewCookieSigner(cookieKey)
	sessions := auth.NewSessionStore(db.RawDB())

	clientID := os.Getenv("GITHUB_CLIENT_ID")
	clientSecret := os.Getenv("GITHUB_CLIENT_SECRET")
	redirectURL := "http://localhost" + addr + "/callback"

	provider := auth.NewGitHubProvider(clientID, clientSecret, redirectURL)

	ah := &auth.Handlers{
		Provider:    provider,
		Sessions:    sessions,
		Signer:      signer,
		CookieName:  "session",
		StateCookie: "oauth_state",
		Secure:      false,
		EnsureAuthor: func(ctx context.Context, user auth.GitHubUser) (int64, error) {
			return db.InsertAuthor(ctx, user.Username, user.Name, user.AvatarURL, user.ProfileURL)
		},
	}

	srv := web.NewServer(db, blobs, ah, sessions, signer, "session")

	fmt.Printf("listening on %s\n", addr)
	if err := http.ListenAndServe(addr, srv.Routes()); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
