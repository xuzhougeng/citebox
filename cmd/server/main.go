package main

import (
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/config"
	"github.com/xuzhougeng/citebox/internal/handler"
	"github.com/xuzhougeng/citebox/internal/logging"
	"github.com/xuzhougeng/citebox/internal/middleware"
	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/service"
)

func main() {
	logger := logging.New()
	slog.SetDefault(logger)

	cfg := config.Load()

	repo, err := repository.NewLibraryRepository(cfg.DatabasePath)
	if err != nil {
		logger.Error("failed to initialize database", "code", apperr.CodeOf(err), "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := repo.Close(); err != nil {
			logger.Warn("failed to close database", "error", err)
		}
	}()

	librarySvc, err := service.NewLibraryService(
		repo,
		cfg,
		service.WithLogger(logger.With("component", "library_service")),
	)
	if err != nil {
		logger.Error("failed to initialize library service", "code", apperr.CodeOf(err), "error", err)
		os.Exit(1)
	}
	aiSvc := service.NewAIService(repo, cfg, logger.With("component", "ai_service"))
	paperHandler := handler.NewPaperHandler(librarySvc)
	figureHandler := handler.NewFigureHandler(librarySvc)
	groupHandler := handler.NewGroupHandler(librarySvc)
	tagHandler := handler.NewTagHandler(librarySvc)
	aiHandler := handler.NewAIHandler(aiSvc)
	settingsHandler := handler.NewSettingsHandler(librarySvc)
	databaseHandler := handler.NewDatabaseHandler(librarySvc)
	authHandler := handler.NewAuthHandler(librarySvc)

	mux := http.NewServeMux()

	mux.HandleFunc("/api/papers", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			paperHandler.List(w, r)
		case http.MethodPost:
			paperHandler.Upload(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/papers/purge", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			paperHandler.Purge(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/papers/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/reextract") {
			paperHandler.Reextract(w, r)
			return
		}

		switch r.Method {
		case http.MethodGet:
			paperHandler.GetByID(w, r)
		case http.MethodPut:
			paperHandler.Update(w, r)
		case http.MethodDelete:
			paperHandler.Delete(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/figures", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			figureHandler.List(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/groups", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			groupHandler.List(w, r)
		case http.MethodPost:
			groupHandler.Create(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/groups/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			groupHandler.Update(w, r)
		case http.MethodDelete:
			groupHandler.Delete(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			tagHandler.List(w, r)
		case http.MethodPost:
			tagHandler.Create(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/tags/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			tagHandler.Update(w, r)
		case http.MethodDelete:
			tagHandler.Delete(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/ai/settings", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			aiHandler.GetSettings(w, r)
		case http.MethodPut:
			aiHandler.UpdateSettings(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/ai/read", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			aiHandler.Read(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/ai/read/stream", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			aiHandler.ReadStream(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/settings/extractor", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			settingsHandler.GetExtractorSettings(w, r)
		case http.MethodPut:
			settingsHandler.UpdateExtractorSettings(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/auth/settings", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			authHandler.GetAuthSettings(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/auth/change-password", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			authHandler.ChangePassword(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			authHandler.Logout(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/database/export", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			databaseHandler.Export(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/database/import", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			databaseHandler.Import(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.Handle("/files/papers/", http.StripPrefix("/files/papers/", http.FileServer(http.Dir(cfg.PapersDir()))))
	mux.Handle("/files/figures/", http.StripPrefix("/files/figures/", http.FileServer(http.Dir(cfg.FiguresDir()))))

	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			http.ServeFile(w, r, "web/index.html")
			return
		}
		if r.URL.Path == "/upload" || r.URL.Path == "/upload.html" {
			http.ServeFile(w, r, "web/upload.html")
			return
		}
		if r.URL.Path == "/figures" || r.URL.Path == "/figures.html" {
			http.ServeFile(w, r, "web/figures.html")
			return
		}
		if r.URL.Path == "/groups" || r.URL.Path == "/groups.html" {
			http.ServeFile(w, r, "web/groups.html")
			return
		}
		if r.URL.Path == "/tags" || r.URL.Path == "/tags.html" {
			http.ServeFile(w, r, "web/tags.html")
			return
		}
		if r.URL.Path == "/ai" || r.URL.Path == "/ai.html" {
			http.ServeFile(w, r, "web/ai.html")
			return
		}
		if r.URL.Path == "/settings" || r.URL.Path == "/settings.html" {
			http.ServeFile(w, r, "web/settings.html")
			return
		}
		if r.URL.Path == "/login" || r.URL.Path == "/login.html" {
			http.ServeFile(w, r, "web/login.html")
			return
		}
		http.NotFound(w, r)
	})

	authenticated := middleware.BasicAuthWithService(mux, librarySvc)
	logged := middleware.RequestLogger(authenticated, logger.With("component", "http"))
	handler := corsMiddleware(logged)

	logger.Info("server starting",
		"port", cfg.ServerPort,
		"storage_dir", cfg.StorageDir,
		"database_path", cfg.DatabasePath,
	)
	if strings.TrimSpace(cfg.ExtractorURL) == "" {
		logger.Info("pdf extractor env config not set; runtime settings page can enable it")
	} else {
		logger.Info("pdf extractor enabled", "extract_url", cfg.EffectiveExtractorURL())
		if jobsURL := strings.TrimSpace(cfg.EffectiveExtractorJobsURL()); jobsURL != "" {
			logger.Info("pdf extractor jobs enabled", "jobs_url", jobsURL)
		}
	}

	if err := http.ListenAndServe(":"+cfg.ServerPort, handler); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
