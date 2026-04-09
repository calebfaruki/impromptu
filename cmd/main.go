package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/calebfaruki/impromptu/internal/commands"
	"github.com/calebfaruki/impromptu/internal/contentcheck"
	"github.com/calebfaruki/impromptu/internal/index"
	"github.com/calebfaruki/impromptu/internal/indexdb"
	"github.com/calebfaruki/impromptu/internal/promptfile"
	"github.com/calebfaruki/impromptu/internal/pull"
	"github.com/calebfaruki/impromptu/internal/sigstore"
	"github.com/calebfaruki/impromptu/web"
	_ "modernc.org/sqlite"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] == "--help" || os.Args[1] == "-h" {
		printHelp()
		os.Exit(0)
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
	var dsn string
	if dev {
		os.MkdirAll("./tmp", 0755)
		dsn = "./tmp/impromptu.db"
	} else {
		dsn = requireEnv("IMPROMPTU_DB_PATH")
	}

	// Open via index.Open for WAL mode + migrations
	legacyDB, err := index.Open(dsn)
	if err != nil {
		fatal("opening database: %v", err)
	}
	defer legacyDB.Close()

	migrations := os.DirFS(".")
	if err := index.Migrate(context.Background(), legacyDB, migrations); err != nil {
		fatal("running migrations: %v", err)
	}

	// Wrap the raw *sql.DB with indexdb
	idx := indexdb.New(legacyDB.RawDB())

	// Verifier
	rekorURL := envOr("REKOR_URL", sigstore.DefaultRekorURL)
	verifier := sigstore.NewRekorVerifier(rekorURL)

	srv := web.NewServer(idx, verifier)

	httpSrv := &http.Server{
		Addr:    addr,
		Handler: srv.Routes(),
	}

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
	inline := false
	var gitURL, ref, release, path, asset, alias string

	args := os.Args[2:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--force":
			force = true
		case "--yes":
			yes = true
		case "--inline":
			inline = true
		case "--git":
			if i+1 < len(args) {
				i++
				gitURL = args[i]
			}
		case "--ref":
			if i+1 < len(args) {
				i++
				ref = args[i]
			}
		case "--release":
			if i+1 < len(args) {
				i++
				release = args[i]
			}
		case "--path":
			if i+1 < len(args) {
				i++
				path = args[i]
			}
		case "--asset":
			if i+1 < len(args) {
				i++
				asset = args[i]
			}
		case "--as":
			if i+1 < len(args) {
				i++
				alias = args[i]
			}
		}
	}

	indexURL := envOr("IMPROMPTU_INDEX", "http://localhost:8080")
	rekorURL := envOr("REKOR_URL", sigstore.DefaultRekorURL)
	cfg := pull.Config{
		Dir:      dir,
		Force:    force,
		Yes:      yes,
		IndexURL: indexURL,
		Verifier: sigstore.NewRekorVerifier(rekorURL),
		Searcher: sigstore.NewRekorSearcher(rekorURL),
		Progress: os.Stderr,
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
	if gitURL != "" {
		src, srcErr := promptfile.SourceFromFlags(gitURL, ref, release, path, asset, inline)
		if srcErr != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", srcErr)
			os.Exit(1)
		}
		result, err = pull.InlinePull(context.Background(), cfg, src, alias)
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
	indexURL := envOr("IMPROMPTU_INDEX", "http://localhost:8080")

	results, err := commands.Search(context.Background(), indexURL, query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if len(results) == 0 {
		fmt.Println("No results found.")
		return
	}
	for _, r := range results {
		signer := ""
		if r.SignerIdentity != "" {
			signer = " (signed by " + r.SignerIdentity + ")"
		}
		fmt.Printf("  %s%s\n", r.SourceURL, signer)
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
	rekorUpdateURL := envOr("REKOR_URL", sigstore.DefaultRekorURL)
	cfg := pull.Config{
		Dir: dir, Force: force, Yes: yes, RegistryURL: registryURL,
		Verifier: sigstore.NewRekorVerifier(rekorUpdateURL),
		Searcher: sigstore.NewRekorSearcher(rekorUpdateURL),
		Progress: os.Stderr,
		Confirm: func(summary string) bool {
			fmt.Print(summary)
			fmt.Print("Continue? [y/N] ")
			var answer string
			fmt.Scanln(&answer)
			return strings.ToLower(answer) == "y"
		},
	}

	result, err := commands.Update(context.Background(), cfg, names...)
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

func printHelp() {
	fmt.Print(`impromptu -- secure prompt distribution

Commands:
  init       Create a new Promptfile in the current directory
  pull       Fetch prompt dependencies (or add a new one with --git)
  search     Search the index for prompts
  update     Check for newer versions of dependencies
  remove     Remove a dependency

Pull flags:
  --git <url>       Git repository URL
  --ref <ref>       Clone mode: tag, branch, or commit SHA (auto-detected)
  --release <tag>   Release mode: download signed release assets
  --path <dir>      Subdirectory within git repo (clone mode only)
  --asset <file>    Non-standard asset filename (release mode only)
  --inline          Place single-file prompt in cwd instead of subdirectory
  --as <alias>      Override default alias name
  --force           Bypass security checks (unsigned, mutable refs)
  --yes             Skip confirmation prompts (CI mode)

Environment:
  IMPROMPTU_INDEX   Index server URL (default: http://localhost:8080)
`)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
