package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/calebfaruki/impromptu/internal/auth"
	"github.com/calebfaruki/impromptu/internal/commands"
	"github.com/calebfaruki/impromptu/internal/contentcheck"
	"github.com/calebfaruki/impromptu/internal/index"
	"github.com/calebfaruki/impromptu/internal/pull"
	"github.com/calebfaruki/impromptu/internal/registry"
	"github.com/calebfaruki/impromptu/internal/sigstore"
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
		dev := len(os.Args) > 2 && os.Args[2] == "--dev"
		runServe(dev)
	case "pull":
		runPull()
	case "init":
		runInit()
	case "search":
		runSearch()
	case "update":
		runUpdate()
	case "remove":
		runRemove()
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

func runServe(dev bool) {
	port := envOr("PORT", "8080")
	addr := ":" + port

	// Database
	var db *index.DB
	var err error
	if dev {
		os.MkdirAll("./tmp", 0755)
		db, err = index.Open("./tmp/impromptu.db")
	} else {
		dbPath := requireEnv("IMPROMPTU_DB_PATH")
		db, err = index.Open(dbPath)
	}
	if err != nil {
		fatal("opening database: %v", err)
	}
	defer db.Close()

	migrations := os.DirFS(".")
	if err := index.Migrate(context.Background(), db, migrations); err != nil {
		fatal("running migrations: %v", err)
	}

	// Blob store
	var blobs registry.BlobStore
	if dev {
		blobs, err = registry.NewFilesystemStore("./tmp/blobs")
		if err != nil {
			fatal("creating blob store: %v", err)
		}
	} else {
		blobs, err = registry.NewR2Store(
			requireEnv("R2_ENDPOINT"),
			requireEnv("R2_ACCESS_KEY_ID"),
			requireEnv("R2_SECRET_ACCESS_KEY"),
			requireEnv("R2_BUCKET"),
		)
		if err != nil {
			fatal("creating R2 store: %v", err)
		}
	}

	// Cookie signing
	var cookieKey []byte
	secure := false
	if dev {
		cookieKey = []byte("dev-mode-key-32-bytes-long-ok!!!")
	} else {
		keyHex := requireEnv("COOKIE_KEY")
		cookieKey, err = hex.DecodeString(keyHex)
		if err != nil {
			fatal("decoding COOKIE_KEY: %v", err)
		}
		secure = true
	}
	signer := auth.NewCookieSigner(cookieKey)
	sessions := auth.NewSessionStore(db.RawDB())

	// OAuth
	clientID := envOr("GITHUB_CLIENT_ID", "")
	clientSecret := envOr("GITHUB_CLIENT_SECRET", "")
	var redirectURL string
	if dev {
		redirectURL = "http://localhost" + addr + "/callback"
	} else {
		domain := requireEnv("DOMAIN")
		redirectURL = "https://" + domain + "/callback"
	}
	provider := auth.NewGitHubProvider(clientID, clientSecret, redirectURL)

	ah := &auth.Handlers{
		Provider:    provider,
		Sessions:    sessions,
		Signer:      signer,
		CookieName:  "session",
		StateCookie: "oauth_state",
		Secure:      secure,
		EnsureAuthor: func(ctx context.Context, user auth.GitHubUser) (int64, error) {
			return db.InsertAuthor(ctx, user.Username, user.Name, user.AvatarURL, user.ProfileURL)
		},
	}

	// Artifact signer
	var artSigner sigstore.Signer = &sigstore.FakeSigner{}

	srv := web.NewServer(db, blobs, artSigner, ah, sessions, signer, "session")

	httpSrv := &http.Server{
		Addr:    addr,
		Handler: srv.Routes(),
	}

	// Graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		fmt.Printf("listening on %s (dev=%v)\n", addr, dev)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fatal("server error: %v", err)
		}
	}()

	<-ctx.Done()
	fmt.Println("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	httpSrv.Shutdown(shutdownCtx)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		fatal("required environment variable %s is not set", key)
	}
	return v
}

func runPull() {
	dir, _ := os.Getwd()
	force := false
	yes := false
	var inlineRef, alias string

	args := os.Args[2:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--force":
			force = true
		case "--yes":
			yes = true
		case "--as":
			if i+1 < len(args) {
				i++
				alias = args[i]
			}
		default:
			if !strings.HasPrefix(args[i], "-") {
				inlineRef = args[i]
			}
		}
	}

	registryURL := envOr("IMPROMPTU_REGISTRY", "http://localhost:8080")
	cfg := pull.Config{
		Dir:         dir,
		Force:       force,
		Yes:         yes,
		RegistryURL: registryURL,
		Verifier:    &sigstore.FakeVerifier{},
		Confirm: func(summary string) bool {
			fmt.Print(summary)
			fmt.Print("Continue? [y/N] ")
			var answer string
			fmt.Scanln(&answer)
			return strings.ToLower(answer) == "y" || strings.ToLower(answer) == "yes"
		},
	}

	var result *pull.Result
	var err error
	if inlineRef != "" {
		result, err = pull.InlinePull(context.Background(), cfg, inlineRef, alias)
	} else {
		result, err = pull.Pull(context.Background(), cfg)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	for _, w := range result.Warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w)
	}
	if len(result.Added) > 0 {
		fmt.Printf("Added: %s\n", strings.Join(result.Added, ", "))
	}
	if len(result.Removed) > 0 {
		fmt.Printf("Removed: %s\n", strings.Join(result.Removed, ", "))
	}
	if len(result.Added) == 0 && len(result.Removed) == 0 {
		fmt.Println("Everything up to date.")
	}
}

func runInit() {
	dir, _ := os.Getwd()
	if err := commands.Init(dir); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Promptfile created.")
}

func runSearch() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "usage: impromptu search <query>\n")
		os.Exit(1)
	}
	query := strings.Join(os.Args[2:], " ")
	registryURL := envOr("IMPROMPTU_REGISTRY", "http://localhost:8080")

	results, err := commands.Search(context.Background(), registryURL, query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if len(results) == 0 {
		fmt.Println("No results found.")
		return
	}
	for _, r := range results {
		fmt.Printf("  %s/%s  %s\n", r.Author, r.Name, r.Description)
	}
}

func runUpdate() {
	dir, _ := os.Getwd()
	force := false
	yes := false
	var names []string

	for _, arg := range os.Args[2:] {
		switch arg {
		case "--force":
			force = true
		case "--yes":
			yes = true
		default:
			if !strings.HasPrefix(arg, "-") {
				names = append(names, arg)
			}
		}
	}

	registryURL := envOr("IMPROMPTU_REGISTRY", "http://localhost:8080")
	cfg := pull.Config{
		Dir: dir, Force: force, Yes: yes, RegistryURL: registryURL,
		Verifier: &sigstore.FakeVerifier{},
		Confirm: func(summary string) bool {
			fmt.Print(summary)
			fmt.Print("Continue? [y/N] ")
			var answer string
			fmt.Scanln(&answer)
			return strings.ToLower(answer) == "y"
		},
	}

	result, err := commands.Update(context.Background(), cfg, &sigstore.FakeVerifier{}, names...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	for _, w := range result.Warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w)
	}
	if len(result.Added) > 0 {
		fmt.Printf("Updated: %s\n", strings.Join(result.Added, ", "))
	} else {
		fmt.Println("Everything up to date.")
	}
}

func runRemove() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "usage: impromptu remove <alias>\n")
		os.Exit(1)
	}
	dir, _ := os.Getwd()
	alias := os.Args[2]

	if err := commands.Remove(dir, alias); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Removed %s.\n", alias)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
