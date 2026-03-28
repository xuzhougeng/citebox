package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/config"
	"github.com/xuzhougeng/citebox/internal/handler"
	"github.com/xuzhougeng/citebox/internal/logging"
	"github.com/xuzhougeng/citebox/internal/middleware"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/service"
)

type Options struct {
	Config  *config.Config
	Logger  *slog.Logger
	WebRoot string
}

type Server struct {
	cfg          *config.Config
	logger       *slog.Logger
	repo         *repository.LibraryRepository
	librarySvc   *service.LibraryService
	httpServer   *http.Server
	bridgeCancel context.CancelFunc
	bridgeDone   chan struct{}
}

func NewServer(opts Options) (*Server, error) {
	cfg := opts.Config
	if cfg == nil {
		cfg = config.Load()
	}

	logger := opts.Logger
	if logger == nil {
		logger = logging.New()
	}

	webRoot := strings.TrimSpace(opts.WebRoot)
	if webRoot == "" {
		webRoot = "web"
	}

	absoluteWebRoot, err := filepath.Abs(webRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve web root: %w", err)
	}
	if err := validateWebRoot(absoluteWebRoot); err != nil {
		return nil, err
	}
	if err := registerWebAssetMIMETypes(); err != nil {
		return nil, fmt.Errorf("register web asset mime types: %w", err)
	}

	repo, err := repository.NewLibraryRepository(cfg.DatabasePath)
	if err != nil {
		return nil, err
	}

	librarySvc, err := service.NewLibraryService(
		repo,
		cfg,
		service.WithLogger(logger.With("component", "library_service")),
	)
	if err != nil {
		_ = repo.Close()
		return nil, err
	}

	aiSvc := service.NewAIService(repo, cfg, logger.With("component", "ai_service"))

	httpServer := &http.Server{
		Addr:    ":" + cfg.ServerPort,
		Handler: buildHandler(cfg, logger, librarySvc, aiSvc, repo, absoluteWebRoot),
	}

	server := &Server{
		cfg:        cfg,
		logger:     logger,
		repo:       repo,
		librarySvc: librarySvc,
		httpServer: httpServer,
	}

	logger.Info("resolved web root", "web_root", absoluteWebRoot)

	bridgeCtx, bridgeCancel := context.WithCancel(context.Background())
	bridgeDone := make(chan struct{})
	bridge := service.NewWeixinIMBridge(
		librarySvc,
		aiSvc,
		logger.With("component", "weixin_im_bridge"),
		cfg.StorageDir,
	)

	server.bridgeCancel = bridgeCancel
	server.bridgeDone = bridgeDone

	go func() {
		defer close(bridgeDone)
		if err := bridge.Run(bridgeCtx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("weixin IM bridge stopped", "error", err)
		}
	}()

	return server, nil
}

func (s *Server) ListenAndServe() error {
	addr := ":" + s.cfg.ServerPort
	s.logStartup(addr)

	err := s.httpServer.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) Serve(listener net.Listener) error {
	s.logStartup(listener.Addr().String())

	err := s.httpServer.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) Close() error {
	if s.bridgeCancel != nil {
		s.bridgeCancel()
	}
	if s.bridgeDone != nil {
		select {
		case <-s.bridgeDone:
		case <-time.After(3 * time.Second):
			s.logger.Warn("timeout waiting for weixin IM bridge shutdown")
		}
	}

	var serverErr error
	if s.httpServer != nil {
		serverErr = s.httpServer.Close()
		if errors.Is(serverErr, http.ErrServerClosed) {
			serverErr = nil
		}
	}

	var repoErr error
	if s.repo != nil {
		repoErr = s.repo.Close()
	}

	return errors.Join(serverErr, repoErr)
}

func (s *Server) logStartup(addr string) {
	s.logger.Info("server starting",
		"addr", addr,
		"storage_dir", s.cfg.StorageDir,
		"database_path", s.cfg.DatabasePath,
	)
	if strings.TrimSpace(s.cfg.ExtractorURL) == "" {
		s.logger.Info("pdf extractor env config not set; runtime settings page can enable it")
		return
	}

	s.logger.Info("pdf extractor enabled", "extract_url", s.cfg.EffectiveExtractorURL())
	if jobsURL := strings.TrimSpace(s.cfg.EffectiveExtractorJobsURL()); jobsURL != "" {
		s.logger.Info("pdf extractor jobs enabled", "jobs_url", jobsURL)
	}
}

func buildHandler(
	cfg *config.Config,
	logger *slog.Logger,
	librarySvc *service.LibraryService,
	aiSvc *service.AIService,
	repo *repository.LibraryRepository,
	webRoot string,
) http.Handler {
	versionSvc := service.NewVersionService()
	paperHandler := handler.NewPaperHandler(librarySvc)
	figureHandler := handler.NewFigureHandler(librarySvc)
	paletteHandler := handler.NewPaletteHandler(librarySvc)
	groupHandler := handler.NewGroupHandler(librarySvc)
	tagHandler := handler.NewTagHandler(librarySvc)
	aiHandler := handler.NewAIHandler(aiSvc)
	settingsHandler := handler.NewSettingsHandler(librarySvc, versionSvc)
	wolaiHandler := handler.NewWolaiHandler(librarySvc)
	databaseHandler := handler.NewDatabaseHandler(librarySvc)
	sessionManager := service.NewSessionManager(24 * time.Hour)
	authHandler := handler.NewAuthHandler(librarySvc, sessionManager)

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

	mux.HandleFunc("/api/papers/import-by-doi", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			paperHandler.ImportByDOI(w, r)
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
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/refresh-doi-metadata") {
			paperHandler.RefreshDOIMetadata(w, r)
			return
		}
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/reextract") {
			paperHandler.Reextract(w, r)
			return
		}
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/pdf-text") {
			paperHandler.UpdatePDFText(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/manual-extraction") {
			switch r.Method {
			case http.MethodGet:
				paperHandler.GetManualExtractionWorkspace(w, r)
			case http.MethodPost:
				paperHandler.ManualExtract(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/manual-preview") {
			paperHandler.ManualPreview(w, r)
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

	mux.HandleFunc("/api/figures/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/image") {
			switch r.Method {
			case http.MethodGet:
				figureHandler.ServeImage(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}
		if strings.HasSuffix(r.URL.Path, "/palette") {
			switch r.Method {
			case http.MethodPost:
				figureHandler.CreatePalette(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}
		if strings.HasSuffix(r.URL.Path, "/subfigures") {
			switch r.Method {
			case http.MethodPost:
				figureHandler.CreateSubfigures(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}

		switch r.Method {
		case http.MethodPut:
			figureHandler.Update(w, r)
		case http.MethodDelete:
			figureHandler.Delete(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/palettes", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			paletteHandler.List(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/palettes/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodDelete:
			paletteHandler.Delete(w, r)
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

	mux.HandleFunc("/api/ai/settings/defaults", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			aiHandler.GetDefaultSettings(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/ai/settings/models", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			aiHandler.UpdateModelSettings(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/ai/settings/prompts", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			aiHandler.UpdatePromptSettings(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/ai/role-prompts", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			aiHandler.GetRolePrompts(w, r)
		case http.MethodPut:
			aiHandler.UpdateRolePrompts(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/ai/prompt-presets", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			aiHandler.GetRolePrompts(w, r)
		case http.MethodPut:
			aiHandler.UpdateRolePrompts(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/ai/settings/check-model", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			aiHandler.CheckModel(w, r)
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

	mux.HandleFunc("/api/ai/translate", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			aiHandler.Translate(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/ai/detect-figure-regions", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			aiHandler.DetectFigureRegions(w, r)
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

	mux.HandleFunc("/api/ai/read/export", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			aiHandler.ExportReadMarkdown(w, r)
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

	mux.HandleFunc("/api/settings/wolai", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			settingsHandler.GetWolaiSettings(w, r)
		case http.MethodPut:
			settingsHandler.UpdateWolaiSettings(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/settings/wolai/test", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			settingsHandler.TestWolaiSettings(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/settings/wolai/test-page", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			settingsHandler.InsertWolaiTestPage(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/settings/desktop-close", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			settingsHandler.GetDesktopCloseSettings(w, r)
		case http.MethodPut:
			settingsHandler.UpdateDesktopCloseSettings(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/settings/weixin-bridge", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			settingsHandler.GetWeixinBridgeSettings(w, r)
		case http.MethodPut:
			settingsHandler.UpdateWeixinBridgeSettings(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/settings/weixin-bridge/daily-recommendation/test", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			settingsHandler.TestWeixinDailyRecommendation(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/settings/tts", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			settingsHandler.GetTTSSettings(w, r)
		case http.MethodPut:
			settingsHandler.UpdateTTSSettings(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/settings/tts/test", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			settingsHandler.TestTTS(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/settings/version", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			settingsHandler.GetVersionStatus(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/wolai/papers/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/notes") {
			wolaiHandler.SavePaperNote(w, r)
			return
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/api/wolai/figures/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/notes") {
			wolaiHandler.SaveFigureNote(w, r)
			return
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/api/auth/settings", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			authHandler.GetAuthSettings(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/auth/weixin/bind", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			authHandler.StartWeixinBinding(w, r)
		case http.MethodDelete:
			authHandler.UnbindWeixin(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/auth/weixin/bind/status", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			authHandler.GetWeixinBindingStatus(w, r)
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

	mux.HandleFunc("/api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			authHandler.Login(w, r)
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
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(filepath.Join(webRoot, "static")))))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/", "/index.html":
			http.ServeFile(w, r, filepath.Join(webRoot, "index.html"))
		case "/library", "/library.html":
			http.ServeFile(w, r, filepath.Join(webRoot, "library.html"))
		case "/guide", "/guide.html":
			http.ServeFile(w, r, filepath.Join(webRoot, "guide.html"))
		case "/upload", "/upload.html":
			http.ServeFile(w, r, filepath.Join(webRoot, "upload.html"))
		case "/manual", "/manual.html":
			http.ServeFile(w, r, filepath.Join(webRoot, "manual.html"))
		case "/viewer", "/viewer.html":
			http.ServeFile(w, r, filepath.Join(webRoot, "viewer.html"))
		case "/figures", "/figures.html":
			http.ServeFile(w, r, filepath.Join(webRoot, "figures.html"))
		case "/palettes", "/palettes.html":
			http.ServeFile(w, r, filepath.Join(webRoot, "palettes.html"))
		case "/groups", "/groups.html":
			http.ServeFile(w, r, filepath.Join(webRoot, "groups.html"))
		case "/tags", "/tags.html":
			http.ServeFile(w, r, filepath.Join(webRoot, "tags.html"))
		case "/notes", "/notes.html":
			http.ServeFile(w, r, filepath.Join(webRoot, "notes.html"))
		case "/ai", "/ai.html":
			http.ServeFile(w, r, filepath.Join(webRoot, "ai.html"))
		case "/settings", "/settings.html":
			http.ServeFile(w, r, filepath.Join(webRoot, "settings.html"))
		case "/login", "/login.html":
			http.ServeFile(w, r, filepath.Join(webRoot, "login.html"))
		default:
			http.NotFound(w, r)
		}
	})

	authMiddleware := middleware.AuthMiddleware(sessionManager, []middleware.PublicPath{
		{Path: "/login", Prefix: false},
		{Path: "/login.html", Prefix: false},
		{Path: "/api/auth/login", Prefix: false},
		{Path: "/static/", Prefix: true},
	}, cfg.DisableAuth)

	authenticated := authMiddleware(mux)
	logged := middleware.RequestLogger(authenticated, logger.With("component", "http"))
	return corsMiddleware(logged)
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

func validateWebRoot(webRoot string) error {
	indexPath := filepath.Join(webRoot, "index.html")
	info, err := os.Stat(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return apperr.Wrap(apperr.CodeInvalidArgument, "web 资源目录缺少 index.html", err)
		}
		return fmt.Errorf("stat web root: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("web root is not a file-backed asset directory: %s", indexPath)
	}
	return nil
}

func (s *Server) GetDesktopCloseSettings() (*model.DesktopCloseSettings, error) {
	if s == nil || s.librarySvc == nil {
		return &model.DesktopCloseSettings{Action: model.DesktopCloseActionAsk}, nil
	}
	return s.librarySvc.GetDesktopCloseSettings()
}

func (s *Server) UpdateDesktopCloseSettings(input model.DesktopCloseSettings) (*model.DesktopCloseSettings, error) {
	if s == nil || s.librarySvc == nil {
		settings := &model.DesktopCloseSettings{Action: model.NormalizeDesktopCloseAction(input.Action)}
		return settings, nil
	}
	return s.librarySvc.UpdateDesktopCloseSettings(input)
}
