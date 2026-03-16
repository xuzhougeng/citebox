package main

import (
	"log"
	"net/http"
	"strings"

	"paper_image_db/internal/config"
	"paper_image_db/internal/handler"
	"paper_image_db/internal/middleware"
	"paper_image_db/internal/repository"
	"paper_image_db/internal/service"
)

func main() {
	cfg := config.Load()

	repo, err := repository.NewLibraryRepository(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer repo.Close()

	librarySvc := service.NewLibraryService(repo, cfg)
	paperHandler := handler.NewPaperHandler(librarySvc)
	figureHandler := handler.NewFigureHandler(librarySvc)
	groupHandler := handler.NewGroupHandler(librarySvc)
	tagHandler := handler.NewTagHandler(librarySvc)

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
		http.NotFound(w, r)
	})

	authenticated := middleware.BasicAuth(mux, cfg)
	handler := corsMiddleware(authenticated)

	log.Printf("Server starting on port %s...", cfg.ServerPort)
	log.Printf("Storage directory: %s", cfg.StorageDir)
	log.Printf("Database path: %s", cfg.DatabasePath)
	if strings.TrimSpace(cfg.ExtractorURL) == "" {
		log.Printf("PDF extractor: disabled (set PDF_EXTRACTOR_URL to enable parsing)")
	} else {
		log.Printf("PDF extractor: %s", cfg.EffectiveExtractorURL())
		if jobsURL := strings.TrimSpace(cfg.EffectiveExtractorJobsURL()); jobsURL != "" {
			log.Printf("PDF extractor jobs: %s", jobsURL)
		}
	}

	if err := http.ListenAndServe(":"+cfg.ServerPort, handler); err != nil {
		log.Fatalf("Server failed: %v", err)
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
