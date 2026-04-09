package web

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/calebfaruki/impromptu/internal/indexdb"
	"github.com/calebfaruki/impromptu/internal/sigstore"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

// Server holds dependencies for all HTTP handlers.
type Server struct {
	idx         *indexdb.DB
	verifier    sigstore.Verifier
	probeClient *http.Client
	pages       map[string]*template.Template
}

// NewServer creates a web server with all dependencies wired.
func NewServer(idx *indexdb.DB, verifier sigstore.Verifier) *Server {
	funcMap := template.FuncMap{"highlight": highlight}

	layout := template.Must(
		template.New("layout").Funcs(funcMap).ParseFS(templateFS, "templates/layout.html"),
	)

	pageNames := []string{
		"home.html", "search.html", "prompt_index.html",
		"404.html",
	}

	pages := make(map[string]*template.Template, len(pageNames))
	for _, name := range pageNames {
		pages[name] = template.Must(
			template.Must(layout.Clone()).ParseFS(templateFS, "templates/"+name),
		)
	}

	return &Server{
		idx:      idx,
		verifier: verifier,
		pages:    pages,
	}
}

// Routes returns the Chi router with all routes configured.
func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", s.HandleHealthz)

	staticSub, _ := fs.Sub(staticFS, "static")
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	r.Get("/", s.HandleHome)
	r.Get("/search", s.HandleSearch)
	r.Get("/prompt/{url}", s.HandlePromptIndex)

	r.Post("/api/index", s.HandleIndexAPI)
	r.Get("/api/search", s.HandleSearchAPI)

	r.NotFound(s.HandleNotFound)

	return r
}
