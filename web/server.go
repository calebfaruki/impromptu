package web

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/calebfaruki/impromptu/internal/auth"
	"github.com/calebfaruki/impromptu/internal/index"
	"github.com/calebfaruki/impromptu/internal/registry"
	"github.com/calebfaruki/impromptu/internal/sigstore"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

// Server holds dependencies for all HTTP handlers.
type Server struct {
	db           *index.DB
	blobs        registry.BlobStore
	artSigner    sigstore.Signer
	authH        *auth.Handlers
	sessions     *auth.SessionStore
	cookieSigner *auth.CookieSigner
	cookie       string
	pages        map[string]*template.Template
}

// NewServer creates a web server with all dependencies wired.
func NewServer(db *index.DB, blobs registry.BlobStore, artSigner sigstore.Signer, ah *auth.Handlers, sessions *auth.SessionStore, cookieSigner *auth.CookieSigner, cookieName string) *Server {
	layout := template.Must(
		template.New("layout").ParseFS(templateFS, "templates/layout.html"),
	)

	pageNames := []string{
		"home.html", "search.html", "author.html",
		"prompt.html", "versions.html", "version.html",
		"dashboard.html", "settings.html", "publish.html",
		"404.html",
	}

	pages := make(map[string]*template.Template, len(pageNames))
	for _, name := range pageNames {
		pages[name] = template.Must(
			template.Must(layout.Clone()).ParseFS(templateFS, "templates/"+name),
		)
	}

	return &Server{
		db:           db,
		blobs:        blobs,
		artSigner:    artSigner,
		authH:        ah,
		sessions:     sessions,
		cookieSigner: cookieSigner,
		cookie:       cookieName,
		pages:        pages,
	}
}

// Routes returns the Chi router with all routes configured.
func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(auth.OptionalAuth(s.sessions, s.cookieSigner, s.cookie))

	// Health check (before auth middleware would be ideal, but OptionalAuth is non-blocking)
	r.Get("/healthz", s.HandleHealthz)

	// Static assets
	staticSub, _ := fs.Sub(staticFS, "static")
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	// Public pages
	r.Get("/", s.HandleHome)
	r.Get("/search", s.HandleSearch)
	r.Get("/login", s.authH.HandleLogin)
	r.Get("/callback", s.authH.HandleCallback)
	r.Get("/logout", s.authH.HandleLogout)

	// API
	r.Get("/api/v1/search", s.HandleSearchAPI)
	r.Get("/api/v1/blobs/{digest}", s.HandleBlobDownload)
	r.Get("/api/v1/prompts/{author}/{name}", s.HandlePromptAPI)
	r.Get("/api/v1/prompts/{author}/{name}/versions", s.HandleVersionsAPI)
	r.Post("/api/v1/publish", s.HandlePublishAPI)

	// Authenticated
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth(s.sessions, s.cookieSigner, s.cookie))
		r.Get("/dashboard/prompts", s.HandleDashboard)
		r.Get("/dashboard/settings", s.HandleDashboardSettings)
		r.Get("/publish", s.HandlePublishForm)
		r.Post("/publish", s.HandlePublish)
	})

	// Dynamic author/prompt routes (registered last)
	r.Get("/{author}", s.HandleAuthor)
	r.Get("/{author}/{name}", s.HandlePrompt)
	r.Get("/{author}/{name}/versions", s.HandlePromptVersions)
	r.Get("/{author}/{name}/v/{version}", s.HandlePromptVersion)

	r.NotFound(s.HandleNotFound)

	return r
}
