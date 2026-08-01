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
	"github.com/xuzhougeng/citebox/internal/service/agent_session"
	"github.com/xuzhougeng/citebox/internal/service/agent_session/commands"
	"github.com/xuzhougeng/citebox/internal/service/ai_assistant"
	"github.com/xuzhougeng/citebox/internal/service/ai_conversation"
	"github.com/xuzhougeng/citebox/internal/service/ai_external"
	"github.com/xuzhougeng/citebox/internal/service/ai_image_gen"
	"github.com/xuzhougeng/citebox/internal/service/daily_figure"
	"github.com/xuzhougeng/citebox/internal/service/pubmed"
	"github.com/xuzhougeng/citebox/internal/service/research"
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
	s2Client     *research.Client
	pubmedClient *pubmed.Client
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

	dbS2APIKey, _ := repo.GetAppSetting("s2_api_key")
	s2APIKey := startupS2APIKey(cfg, dbS2APIKey)
	s2Client := research.NewClient(research.Config{
		APIKey:      s2APIKey,
		MinInterval: research.RateInterval(s2APIKey),
	})

	pubmedSettings, err := librarySvc.GetAIExternalSearchSettings()
	if err != nil {
		_ = repo.Close()
		s2Client.Close()
		return nil, fmt.Errorf("load AI external search settings: %w", err)
	}
	pubmedAPIKey, pubmedEmail, pubmedTool := startupPubMedSettings(cfg, pubmedSettings, os.LookupEnv)
	pubmedClient := pubmed.NewClient(pubmed.Config{
		APIKey:      pubmedAPIKey,
		Email:       pubmedEmail,
		Tool:        pubmedTool,
		MinInterval: pubmed.RateInterval(pubmedAPIKey),
	})

	// Build the shared AI services here (rather than inside buildHandler) so
	// the WeChat bridge can wire them into the agent_session.Service before the
	// HTTP handler is even constructed. The handler then receives the same
	// instances and registers its routes against them.
	aiServices := buildAIServices(cfg, logger, librarySvc, aiSvc, repo, s2Client, pubmedClient)

	httpServer := &http.Server{
		Addr:    ":" + cfg.ServerPort,
		Handler: buildHandlerWithAIServices(cfg, logger, librarySvc, aiSvc, repo, absoluteWebRoot, s2Client, pubmedClient, aiServices),
	}

	server := &Server{
		cfg:          cfg,
		logger:       logger,
		repo:         repo,
		librarySvc:   librarySvc,
		httpServer:   httpServer,
		s2Client:     s2Client,
		pubmedClient: pubmedClient,
	}

	logger.Info("resolved web root", "web_root", absoluteWebRoot)

	// Wire the agent_session façade. Both surfaces (WeChat bridge today,
	// desktop ai_conversation handler tomorrow) route through this Service so
	// they share commands, conversation rows, and a single per-user history
	// window. The surface state file lives next to the WeChat bridge's other
	// per-surface artifacts.
	surfaceStatePath := filepath.Join(cfg.StorageDir, "weixin-bridge", "weixin_surface_state.json")
	surfaceState, err := agent_session.NewSurfaceStateStore(surfaceStatePath)
	if err != nil {
		_ = repo.Close()
		s2Client.Close()
		pubmedClient.Close()
		return nil, fmt.Errorf("init surface state: %w", err)
	}

	registry := commands.NewRegistry(
		&commands.ClearCommand{},
		&commands.ResetCommand{},
		&commands.HelpCommand{},
		&commands.StatusCommand{},
		&commands.NoteCommand{Library: librarySvc},
		&commands.RecentCommand{Lister: librarySvc, Limit: 5},
		&commands.SearchCommand{Lister: librarySvc, Limit: 5},
		&commands.FiguresCommand{Library: librarySvc},
		&commands.RandomCommand{Picker: service.NewRandomFigureAdapter(librarySvc)},
		&commands.PaperCommand{Lookup: librarySvc},
		&commands.FigureCommand{Lookup: service.NewFigureLookupAdapter(repo.Figure)},
		&commands.AskCommand{AI: aiSvc},
		&commands.InterpretCommand{AI: aiSvc, Lookup: service.NewFigureLookupAdapter(repo.Figure)},
		&commands.DOICommand{Importer: service.NewDOIImportAdapter(librarySvc)},
	)
	agentSvc := agent_session.New(
		repo.AIConversation,
		registry,
		agent_session.NewAIConversationAdapter(aiServices.conversation),
		surfaceState,
		logger.With("component", "agent_session"),
	)

	if err := agent_session.MigrateLegacyWeixinContext(
		filepath.Join(cfg.StorageDir, "weixin-bridge", "im_context.json"),
		repo.AIConversation, surfaceState, logger,
	); err != nil {
		logger.Warn("legacy weixin context migration failed", "error", err)
	}

	bridgeCtx, bridgeCancel := context.WithCancel(context.Background())
	bridgeDone := make(chan struct{})
	bridge := service.NewWeixinIMBridge(
		librarySvc,
		aiSvc,
		logger.With("component", "weixin_im_bridge"),
		cfg.StorageDir,
		agentSvc,
		surfaceState,
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

func startupS2APIKey(cfg *config.Config, savedAPIKey string) string {
	if cfg != nil {
		if apiKey := strings.TrimSpace(cfg.S2APIKey); apiKey != "" {
			return apiKey
		}
	}
	return strings.TrimSpace(savedAPIKey)
}

func s2RuntimeSettings(cfg *config.Config) handler.ResearchRuntimeSettings {
	if cfg == nil {
		return handler.ResearchRuntimeSettings{}
	}
	apiKey := strings.TrimSpace(cfg.S2APIKey)
	return handler.ResearchRuntimeSettings{
		APIKey:    apiKey,
		APIKeySet: apiKey != "",
	}
}

func startupPubMedSettings(cfg *config.Config, settings *model.AIExternalSearchSettings, lookupEnv func(string) (string, bool)) (apiKey, email, tool string) {
	runtime := pubmedRuntimeSettings(cfg, lookupEnv)
	if runtime.APIKeySet {
		apiKey = strings.TrimSpace(runtime.APIKey)
	} else if settings != nil {
		apiKey = strings.TrimSpace(settings.PubMedAPIKey)
	}
	if runtime.EmailSet {
		email = strings.TrimSpace(runtime.Email)
	} else if settings != nil {
		email = strings.TrimSpace(settings.PubMedEmail)
	}
	if runtime.ToolSet {
		tool = strings.TrimSpace(runtime.Tool)
	} else if settings != nil {
		tool = strings.TrimSpace(settings.PubMedTool)
	}
	if tool == "" {
		tool = "citebox"
	}
	return apiKey, email, tool
}

func pubmedRuntimeSettings(cfg *config.Config, lookupEnv func(string) (string, bool)) handler.PubMedRuntimeSettings {
	var runtime handler.PubMedRuntimeSettings
	if cfg != nil {
		if apiKey := strings.TrimSpace(cfg.PubMedAPIKey); apiKey != "" {
			runtime.APIKey = apiKey
			runtime.APIKeySet = true
		}
		if email := strings.TrimSpace(cfg.PubMedEmail); email != "" {
			runtime.Email = email
			runtime.EmailSet = true
		}
	}
	if tool, ok := nonemptyEnvValue(lookupEnv, "PUBMED_TOOL"); ok {
		runtime.Tool = tool
		runtime.ToolSet = true
	}
	return runtime
}

func nonemptyEnvValue(lookupEnv func(string) (string, bool), key string) (string, bool) {
	if lookupEnv == nil {
		return "", false
	}
	value, ok := lookupEnv(key)
	value = strings.TrimSpace(value)
	return value, ok && value != ""
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

	if s.s2Client != nil {
		s.s2Client.Close()
	}
	if s.pubmedClient != nil {
		s.pubmedClient.Close()
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

// sharedAIServices bundles the AI-side wiring that both the HTTP handler and
// the WeChat bridge need. Constructing it once in Run lets the bridge wire its
// agent_session.Service against the same ai_conversation.Service that the
// handler uses.
type sharedAIServices struct {
	research     *research.Service
	external     *ai_external.Service
	orchestrator *ai_assistant.Orchestrator
	conversation *ai_conversation.Service
	imageGen     *ai_image_gen.Service
}

func buildAIServices(
	cfg *config.Config,
	logger *slog.Logger,
	librarySvc *service.LibraryService,
	aiSvc *service.AIService,
	repo *repository.LibraryRepository,
	s2Client *research.Client,
	pubmedClient *pubmed.Client,
) sharedAIServices {
	researchAdapter := &research.RepoAdapter{Repo: repo.Research}
	researchSvc := research.NewService(s2Client, researchAdapter, research.ServiceConfig{})
	aiExternalSvc := ai_external.NewService(aiExternalSettingsShim{librarySvc: librarySvc}, map[ai_external.SourceID]ai_external.Searcher{
		ai_external.SourcePubMed:          ai_external.PubMedAdapter{Client: pubmedClient},
		ai_external.SourceSemanticScholar: ai_external.SemanticScholarAdapter{SearchService: researchSvc},
	})
	libraryPlanner := ai_assistant.NewLLMLibrarySearchPlanner(aiSvc, aiSvc)
	libraryClassifier := ai_assistant.NewLLMLibraryPaperClassifier(aiSvc, aiSvc)
	externalPlanner := ai_assistant.NewLLMExternalSearchPlanner(aiSvc, aiSvc)
	externalClassifier := ai_assistant.NewLLMExternalPaperClassifier(aiSvc, aiSvc)
	assistantOrchestrator := ai_assistant.NewOrchestrator(ai_assistant.ToolSet{
		LibrarySearch:  ai_assistant.NewLibrarySearchToolWithAgents(repo.Paper, libraryPlanner, libraryClassifier),
		ExternalSearch: ai_assistant.NewExternalSearchToolWithAgents(aiExternalSvc, externalPlanner, externalClassifier),
		PaperRead:      ai_assistant.NewPaperReadTool(repo.Paper),
		FigureLookup:   ai_assistant.NewFigureLookupToolWithPapers(ai_assistant.NewRepositoryFigureSearcher(repo.Figure), repo.Paper),
	})
	imageStorage := ai_image_gen.NewStorage(cfg.AIGeneratedDir())
	imageGenService := ai_image_gen.NewService(ai_image_gen.ServiceDeps{
		Repo:       aiGeneratedImageRepoAdapter{libRepo: repo},
		Storage:    imageStorage,
		Vision:     visionAdapter{svc: aiSvc},
		API:        ai_image_gen.NewClient(http.DefaultClient),
		LoadInputs: aiSvc.LoadImageGenInputs,
		GetSettings: func() (model.AIImageGenSettings, model.AISettings, error) {
			s, err := aiSvc.GetSettings()
			if err != nil {
				return model.AIImageGenSettings{}, model.AISettings{}, err
			}
			return s.ImageGen, *s, nil
		},
	})
	aiConvService := ai_conversation.New(repo.AIConversation, repo.Paper, aiSvc, aiSvc, aiExternalSvc, logger.With("component", "ai_conversation"), assistantOrchestrator).
		WithImageGen(imageGenService).
		WithImageStorage(imageStorage)

	return sharedAIServices{
		research:     researchSvc,
		external:     aiExternalSvc,
		orchestrator: assistantOrchestrator,
		conversation: aiConvService,
		imageGen:     imageGenService,
	}
}

func buildHandler(
	cfg *config.Config,
	logger *slog.Logger,
	librarySvc *service.LibraryService,
	aiSvc *service.AIService,
	repo *repository.LibraryRepository,
	webRoot string,
	s2Client *research.Client,
	pubmedClient *pubmed.Client,
) http.Handler {
	return buildHandlerWithAIServices(cfg, logger, librarySvc, aiSvc, repo, webRoot, s2Client, pubmedClient,
		buildAIServices(cfg, logger, librarySvc, aiSvc, repo, s2Client, pubmedClient))
}

func buildHandlerWithAIServices(
	cfg *config.Config,
	logger *slog.Logger,
	librarySvc *service.LibraryService,
	aiSvc *service.AIService,
	repo *repository.LibraryRepository,
	webRoot string,
	s2Client *research.Client,
	pubmedClient *pubmed.Client,
	aiServices sharedAIServices,
) http.Handler {
	versionSvc := service.NewVersionService()
	paperHandler := handler.NewPaperHandler(librarySvc)
	figureHandler := handler.NewFigureHandler(librarySvc)
	paletteHandler := handler.NewPaletteHandler(librarySvc)
	groupHandler := handler.NewGroupHandler(librarySvc)
	tagHandler := handler.NewTagHandler(librarySvc)
	aiHandler := handler.NewAIHandler(aiSvc)
	researchSvc := aiServices.research
	aiConversationHandler := handler.NewAIConversationHandler(aiServices.conversation)
	settingsHandler := handler.NewSettingsHandler(librarySvc, versionSvc)
	settingsHandler.SetResearchClient(s2Client, s2RuntimeSettings(cfg))
	settingsHandler.SetPubMedClient(pubmedClient, pubmedRuntimeSettings(cfg, os.LookupEnv))
	wolaiHandler := handler.NewWolaiHandler(librarySvc)
	databaseHandler := handler.NewDatabaseHandler(librarySvc)
	sessionManager := service.NewSessionManager(24 * time.Hour)
	authHandler := handler.NewAuthHandler(librarySvc, sessionManager)
	aiGeneratedImageHandler := handler.NewAIGeneratedImageHandler(repo.AIGeneratedImage, cfg.AIGeneratedDir())

	researchAdapter := &research.RepoAdapter{Repo: repo.Research}
	basket := research.NewBasket(researchAdapter, researchSvc, librarySvcImporterShim{librarySvc})
	researchHandler := handler.NewResearchHandler(researchSvc, basket, nil)
	overviewHandler := handler.NewOverviewHandler(librarySvc, daily_figure.New(repo.Figure), librarySvc)

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

	mux.HandleFunc("/api/pdf-annotations", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			paperHandler.ListPDFAnnotationsGlobal(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/papers/", func(w http.ResponseWriter, r *http.Request) {
		trimmedPath := strings.TrimRight(r.URL.Path, "/")
		if strings.Contains(trimmedPath, "/pdf-annotations") {
			if strings.HasSuffix(trimmedPath, "/pdf-annotations") {
				switch r.Method {
				case http.MethodGet:
					paperHandler.ListPDFAnnotations(w, r)
				case http.MethodPost:
					paperHandler.CreatePDFAnnotation(w, r)
				default:
					http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				}
				return
			}
			switch r.Method {
			case http.MethodDelete:
				paperHandler.DeletePDFAnnotation(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}
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
		if strings.HasSuffix(r.URL.Path, "/transfer-package") {
			switch r.Method {
			case http.MethodGet:
				figureHandler.ExportTransferPackage(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}
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

	mux.HandleFunc("/api/ai/conversations", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		aiConversationHandler.List(w, r)
	})

	mux.HandleFunc("/api/ai/conversations/", func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, "/messages"):
			switch r.Method {
			case http.MethodGet:
				aiConversationHandler.Messages(w, r)
			case http.MethodPost:
				aiConversationHandler.PostMessage(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		case strings.HasSuffix(p, "/export"):
			if r.Method != http.MethodGet {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			aiConversationHandler.Export(w, r)
		case strings.Contains(p, "/papers"):
			aiConversationHandler.Pins(w, r)
		default:
			aiConversationHandler.Detail(w, r)
		}
	})

	mux.HandleFunc("/api/ai-generated-images/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/file") {
			aiGeneratedImageHandler.GetFile(w, r)
			return
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
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
	mux.HandleFunc("/api/ai/tts", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			settingsHandler.SynthesizeTTS(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/overview/summary", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		overviewHandler.Summary(w, r)
	})
	mux.HandleFunc("/api/overview/daily-figure", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		overviewHandler.DailyFigure(w, r)
	})
	mux.HandleFunc("/api/overview/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		overviewHandler.Status(w, r)
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

	mux.HandleFunc("/api/auth/remember-login", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			authHandler.UpdateRememberLogin(w, r)
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

	mux.HandleFunc("/api/research/search", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		researchHandler.Search(w, r)
	})

	mux.HandleFunc("/api/research/autocomplete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		researchHandler.Autocomplete(w, r)
	})

	mux.HandleFunc("/api/research/snippets", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		researchHandler.SnippetSearch(w, r)
	})

	mux.HandleFunc("/api/research/recommendations", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		researchHandler.RecommendationsForList(w, r)
	})

	mux.HandleFunc("/api/research/paper/", func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, "/references"):
			researchHandler.References(w, r)
		case strings.HasSuffix(p, "/citations"):
			researchHandler.Citations(w, r)
		case strings.HasSuffix(p, "/recommendations"):
			researchHandler.Recommendations(w, r)
		default:
			researchHandler.GetPaper(w, r)
		}
	})

	mux.HandleFunc("/api/research/basket", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			researchHandler.BasketList(w, r)
		case http.MethodPost:
			researchHandler.BasketAdd(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/research/basket/", func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch p {
		case "/api/research/basket/import-to-library":
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			researchHandler.BasketImportToLibrary(w, r)
			return
		case "/api/research/basket/export":
			if r.Method != http.MethodGet {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			researchHandler.BasketExport(w, r)
			return
		}
		if r.Method == http.MethodDelete {
			researchHandler.BasketRemove(w, r)
			return
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/api/settings/research", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			settingsHandler.GetResearchSettings(w, r)
		case http.MethodPut:
			settingsHandler.PutResearchSettings(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/settings/ai-external-search", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			settingsHandler.GetAIExternalSearchSettings(w, r)
		case http.MethodPut:
			settingsHandler.PutAIExternalSearchSettings(w, r)
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
		case "/highlights", "/highlights.html":
			http.ServeFile(w, r, filepath.Join(webRoot, "highlights.html"))
		case "/ai", "/ai.html":
			http.ServeFile(w, r, filepath.Join(webRoot, "ai.html"))
		case "/settings", "/settings.html":
			http.ServeFile(w, r, filepath.Join(webRoot, "settings.html"))
		case "/login", "/login.html":
			http.ServeFile(w, r, filepath.Join(webRoot, "login.html"))
		case "/research", "/research.html":
			http.ServeFile(w, r, filepath.Join(webRoot, "research.html"))
		case "/overview", "/overview.html":
			http.ServeFile(w, r, filepath.Join(webRoot, "overview.html"))
		default:
			http.NotFound(w, r)
		}
	})

	authMiddleware := middleware.AuthMiddleware(sessionManager, librarySvc, []middleware.PublicPath{
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
