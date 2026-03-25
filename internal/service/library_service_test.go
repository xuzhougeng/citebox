package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/config"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/weixin"
	wolaiapi "github.com/xuzhougeng/citebox/internal/wolai"
)

func newTestService(t *testing.T) (*LibraryService, *repository.LibraryRepository, *config.Config) {
	t.Helper()

	root := t.TempDir()
	cfg := &config.Config{
		StorageDir:              filepath.Join(root, "storage"),
		DatabasePath:            filepath.Join(root, "library.db"),
		MaxUploadSize:           10 << 20,
		AdminUsername:           "citebox",
		AdminPassword:           "citebox123",
		ExtractorTimeoutSeconds: 1,
		ExtractorPollInterval:   1,
		ExtractorFileField:      "file",
	}

	repo, err := repository.NewLibraryRepository(cfg.DatabasePath)
	if err != nil {
		t.Fatalf("NewLibraryRepository() error = %v", err)
	}
	t.Cleanup(func() {
		_ = repo.Close()
	})

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := NewLibraryService(repo, cfg, WithLogger(logger), WithoutBackgroundJobs())
	if err != nil {
		t.Fatalf("NewLibraryService() error = %v", err)
	}

	return svc, repo, cfg
}

func waitForPaperPDFText(t *testing.T, svc *LibraryService, paperID int64) string {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		paper, err := svc.GetPaper(paperID)
		if err == nil && paper != nil {
			pdfText := strings.TrimSpace(paper.PDFText)
			if pdfText != "" {
				return pdfText
			}
		}
		time.Sleep(20 * time.Millisecond)
	}

	paper, err := svc.GetPaper(paperID)
	if err != nil {
		t.Fatalf("GetPaper() after waiting error = %v", err)
	}
	t.Fatalf("paper %d pdf_text still empty after waiting; paper = %+v", paperID, paper)
	return ""
}

func createTestPaper(t *testing.T, repo *repository.LibraryRepository) *model.Paper {
	t.Helper()

	paper, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Atlas Study",
		OriginalFilename: "atlas-study.pdf",
		StoredPDFName:    "paper_test.pdf",
		FileSize:         512,
		ContentType:      "application/pdf",
		PDFText:          "Atlas full text",
		AbstractText:     "Atlas abstract",
		NotesText:        "Atlas notes",
		PaperNotesText:   "Atlas paper notes",
		ExtractionStatus: "completed",
		Tags: []repository.TagUpsertInput{
			{Name: "Atlas", Color: "#123456"},
		},
		Figures: []repository.FigureUpsertInput{
			{
				Filename:     "figure_test.png",
				OriginalName: "figure-original.png",
				ContentType:  "image/png",
				PageNumber:   1,
				FigureIndex:  1,
				Caption:      "Figure",
			},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}

	return paper
}

type testMultipartFile struct {
	*bytes.Reader
}

func wolaiBlockTypes(blocks []map[string]any) []string {
	types := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if blockType, ok := block["type"].(string); ok {
			types = append(types, blockType)
		}
	}
	return types
}

func wolaiBlockContents(blocks []map[string]any) []string {
	contents := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if content := wolaiBlockTitle(block); content != "" {
			contents = append(contents, content)
		}
	}
	return contents
}

func wolaiBlockTitle(block map[string]any) string {
	switch content := block["content"].(type) {
	case string:
		return strings.TrimSpace(content)
	case map[string]any:
		if title, ok := content["title"].(string); ok {
			return strings.TrimSpace(title)
		}
	}
	return ""
}

func (f *testMultipartFile) Close() error {
	return nil
}

func testPNGDataURL(t *testing.T, width, height int) string {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(10 + x), G: uint8(20 + y), B: 180, A: 255})
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode() error = %v", err)
	}

	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

func useWeixinBindingTestServer(t *testing.T, svc *LibraryService, handler http.Handler) {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	svc.weixinClientFactory = func(token string) weixinBindingClient {
		return weixin.NewClient(server.URL, token, server.Client())
	}
}

type stubWolaiClient struct {
	getBlockFunc            func(id string) (map[string]any, error)
	createBlocksFunc        func(parentID string, blocks any) ([]wolaiapi.CreatedBlock, error)
	createUploadSessionFunc func(input wolaiapi.UploadSessionRequest) (*wolaiapi.UploadSession, error)
	uploadFileFunc          func(session wolaiapi.UploadSession, filename, contentType string, file io.Reader) error
	updateBlockFileFunc     func(blockID, fileID string) error
}

func (c *stubWolaiClient) GetBlock(id string) (map[string]any, error) {
	if c.getBlockFunc != nil {
		return c.getBlockFunc(id)
	}
	return map[string]any{"id": id}, nil
}

func (c *stubWolaiClient) CreateBlocks(parentID string, blocks any) ([]wolaiapi.CreatedBlock, error) {
	if c.createBlocksFunc != nil {
		return c.createBlocksFunc(parentID, blocks)
	}
	return nil, nil
}

func (c *stubWolaiClient) CreateUploadSession(input wolaiapi.UploadSessionRequest) (*wolaiapi.UploadSession, error) {
	if c.createUploadSessionFunc != nil {
		return c.createUploadSessionFunc(input)
	}
	return &wolaiapi.UploadSession{}, nil
}

func (c *stubWolaiClient) UploadFile(session wolaiapi.UploadSession, filename, contentType string, file io.Reader) error {
	if c.uploadFileFunc != nil {
		return c.uploadFileFunc(session, filename, contentType, file)
	}
	return nil
}

func (c *stubWolaiClient) UpdateBlockFile(blockID, fileID string) error {
	if c.updateBlockFileFunc != nil {
		return c.updateBlockFileFunc(blockID, fileID)
	}
	return nil
}

func TestListPapersAppliesDefaultsAndDecoratesURLs(t *testing.T) {
	svc, repo, _ := newTestService(t)
	createTestPaper(t, repo)

	result, err := svc.ListPapers(model.PaperFilter{})
	if err != nil {
		t.Fatalf("ListPapers() error = %v", err)
	}

	if result.Page != 1 || result.PageSize != 12 || result.Total != 1 || result.TotalPages != 1 {
		t.Fatalf("ListPapers() pagination = %+v", result)
	}
	if got := result.Papers[0].PDFURL; got != "/files/papers/paper_test.pdf" {
		t.Fatalf("ListPapers() pdf_url = %q, want %q", got, "/files/papers/paper_test.pdf")
	}
}

func TestGetPaperDecoratesFigureURLs(t *testing.T) {
	svc, repo, _ := newTestService(t)
	paper := createTestPaper(t, repo)

	got, err := svc.GetPaper(paper.ID)
	if err != nil {
		t.Fatalf("GetPaper() error = %v", err)
	}

	if got.PDFURL != "/files/papers/paper_test.pdf" {
		t.Fatalf("GetPaper() pdf_url = %q, want %q", got.PDFURL, "/files/papers/paper_test.pdf")
	}
	if len(got.Figures) != 1 || got.Figures[0].ImageURL != "/files/figures/figure_test.png" {
		t.Fatalf("GetPaper() figures = %+v", got.Figures)
	}
	if got.Figures[0].Source != "auto" {
		t.Fatalf("GetPaper() figure source = %q, want %q", got.Figures[0].Source, "auto")
	}
}

func TestStartWeixinBindingReturnsQRCodeDataURL(t *testing.T) {
	svc, _, _ := newTestService(t)
	useWeixinBindingTestServer(t, svc, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ilink/bot/get_bot_qrcode" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/ilink/bot/get_bot_qrcode")
		}
		if got := r.URL.Query().Get("bot_type"); got != "3" {
			t.Fatalf("bot_type = %q, want %q", got, "3")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ret":                0,
			"qrcode":             "session-123",
			"qrcode_img_content": "https://example.invalid/wechat-bind",
		})
	}))

	result, err := svc.StartWeixinBinding(context.Background())
	if err != nil {
		t.Fatalf("StartWeixinBinding() error = %v", err)
	}
	if result.QRCode != "session-123" {
		t.Fatalf("StartWeixinBinding() qrcode = %q, want %q", result.QRCode, "session-123")
	}
	if !strings.HasPrefix(result.QRCodeDataURL, "data:image/png;base64,") {
		t.Fatalf("StartWeixinBinding() qrcode_data_url = %q, want PNG data URL", result.QRCodeDataURL)
	}
	if result.Status != "wait" {
		t.Fatalf("StartWeixinBinding() status = %q, want %q", result.Status, "wait")
	}
}

func TestGetWeixinBindingStatusConfirmedPersistsBinding(t *testing.T) {
	svc, repo, _ := newTestService(t)
	useWeixinBindingTestServer(t, svc, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ilink/bot/get_qrcode_status" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/ilink/bot/get_qrcode_status")
		}
		if got := r.URL.Query().Get("qrcode"); got != "session-123" {
			t.Fatalf("qrcode = %q, want %q", got, "session-123")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ret":           0,
			"status":        "confirmed",
			"bot_token":     "bot-token-123",
			"baseurl":       "https://ilinkai.weixin.qq.com",
			"ilink_bot_id":  "bot@im.bot",
			"ilink_user_id": "user@im.wechat",
			"message":       "",
		})
	}))

	result, err := svc.GetWeixinBindingStatus(context.Background(), "session-123")
	if err != nil {
		t.Fatalf("GetWeixinBindingStatus() error = %v", err)
	}
	if result.Status != "confirmed" {
		t.Fatalf("GetWeixinBindingStatus() status = %q, want %q", result.Status, "confirmed")
	}
	if !result.Binding.Bound || result.Binding.AccountID != "bot@im.bot" || result.Binding.UserID != "user@im.wechat" {
		t.Fatalf("GetWeixinBindingStatus() binding = %+v, want persisted binding summary", result.Binding)
	}

	settings := svc.GetAuthSettings()
	if !settings.WeixinBinding.Bound || settings.WeixinBinding.AccountID != "bot@im.bot" {
		t.Fatalf("GetAuthSettings() weixin_binding = %+v, want persisted binding", settings.WeixinBinding)
	}

	raw, err := repo.GetAppSetting(weixinBindingKey)
	if err != nil {
		t.Fatalf("GetAppSetting(%q) error = %v", weixinBindingKey, err)
	}
	if !strings.Contains(raw, `"token":"bot-token-123"`) {
		t.Fatalf("saved weixin binding = %q, want token persisted", raw)
	}
}

func TestGetWeixinBindingStatusRejectsEmptyQRCode(t *testing.T) {
	svc, _, _ := newTestService(t)

	_, err := svc.GetWeixinBindingStatus(context.Background(), " ")
	if !apperr.IsCode(err, apperr.CodeInvalidArgument) {
		t.Fatalf("GetWeixinBindingStatus() code = %q, want %q", apperr.CodeOf(err), apperr.CodeInvalidArgument)
	}
}

func TestGetWeixinBridgeSettingsDefaultsToConfig(t *testing.T) {
	svc, _, _ := newTestService(t)

	settings, err := svc.GetWeixinBridgeSettings()
	if err != nil {
		t.Fatalf("GetWeixinBridgeSettings() error = %v", err)
	}
	if settings.Enabled {
		t.Fatalf("GetWeixinBridgeSettings() enabled = %v, want false by default", settings.Enabled)
	}
}

func TestUpdateWeixinBridgeSettingsPersistsAndAppearsInAuthSettings(t *testing.T) {
	svc, repo, _ := newTestService(t)

	settings, err := svc.UpdateWeixinBridgeSettings(model.WeixinBridgeSettings{Enabled: true})
	if err != nil {
		t.Fatalf("UpdateWeixinBridgeSettings() error = %v", err)
	}
	if !settings.Enabled {
		t.Fatalf("UpdateWeixinBridgeSettings() enabled = %v, want true", settings.Enabled)
	}

	reloaded, err := svc.GetWeixinBridgeSettings()
	if err != nil {
		t.Fatalf("GetWeixinBridgeSettings() reload error = %v", err)
	}
	if !reloaded.Enabled {
		t.Fatalf("GetWeixinBridgeSettings() reload enabled = %v, want true", reloaded.Enabled)
	}

	authSettings := svc.GetAuthSettings()
	if !authSettings.WeixinBridge.Enabled {
		t.Fatalf("GetAuthSettings() weixin_bridge = %+v, want enabled", authSettings.WeixinBridge)
	}

	raw, err := repo.GetAppSetting(weixinBridgeSettingsKey)
	if err != nil {
		t.Fatalf("GetAppSetting(%q) error = %v", weixinBridgeSettingsKey, err)
	}
	if !strings.Contains(raw, `"enabled":true`) {
		t.Fatalf("saved weixin bridge settings = %q, want enabled persisted", raw)
	}
}

func TestGetDesktopCloseSettingsDefaultsToAsk(t *testing.T) {
	svc, _, _ := newTestService(t)

	settings, err := svc.GetDesktopCloseSettings()
	if err != nil {
		t.Fatalf("GetDesktopCloseSettings() error = %v", err)
	}
	if settings.Action != model.DesktopCloseActionAsk {
		t.Fatalf("GetDesktopCloseSettings() action = %q, want %q", settings.Action, model.DesktopCloseActionAsk)
	}
}

func TestUpdateDesktopCloseSettingsPersistsAndCanReset(t *testing.T) {
	svc, repo, _ := newTestService(t)

	updated, err := svc.UpdateDesktopCloseSettings(model.DesktopCloseSettings{Action: model.DesktopCloseActionMinimize})
	if err != nil {
		t.Fatalf("UpdateDesktopCloseSettings(minimize) error = %v", err)
	}
	if updated.Action != model.DesktopCloseActionMinimize {
		t.Fatalf("UpdateDesktopCloseSettings(minimize) action = %q, want %q", updated.Action, model.DesktopCloseActionMinimize)
	}

	reloaded, err := svc.GetDesktopCloseSettings()
	if err != nil {
		t.Fatalf("GetDesktopCloseSettings() reload error = %v", err)
	}
	if reloaded.Action != model.DesktopCloseActionMinimize {
		t.Fatalf("GetDesktopCloseSettings() reload action = %q, want %q", reloaded.Action, model.DesktopCloseActionMinimize)
	}

	raw, err := repo.GetAppSetting(desktopCloseSettingsKey)
	if err != nil {
		t.Fatalf("GetAppSetting(%q) error = %v", desktopCloseSettingsKey, err)
	}
	if !strings.Contains(raw, `"action":"minimize"`) {
		t.Fatalf("saved desktop close settings = %q, want minimize persisted", raw)
	}

	reset, err := svc.UpdateDesktopCloseSettings(model.DesktopCloseSettings{Action: model.DesktopCloseActionAsk})
	if err != nil {
		t.Fatalf("UpdateDesktopCloseSettings(ask) error = %v", err)
	}
	if reset.Action != model.DesktopCloseActionAsk {
		t.Fatalf("UpdateDesktopCloseSettings(ask) action = %q, want %q", reset.Action, model.DesktopCloseActionAsk)
	}

	raw, err = repo.GetAppSetting(desktopCloseSettingsKey)
	if err != nil {
		t.Fatalf("GetAppSetting(%q) after reset error = %v", desktopCloseSettingsKey, err)
	}
	if raw != "" {
		t.Fatalf("GetAppSetting(%q) after reset = %q, want empty", desktopCloseSettingsKey, raw)
	}
}

func TestWolaiSettingsPersistAndTestBlockAccess(t *testing.T) {
	svc, repo, _ := newTestService(t)

	var testedBlockID string
	svc.wolaiClientFactory = func(settings model.WolaiSettings) (wolaiClient, error) {
		if settings.Token != "wolai-token" {
			t.Fatalf("wolai token = %q, want %q", settings.Token, "wolai-token")
		}
		if settings.ParentBlockID != "block-123" {
			t.Fatalf("wolai parent_block_id = %q, want %q", settings.ParentBlockID, "block-123")
		}
		if settings.BaseURL != "https://openapi.wolai.com" {
			t.Fatalf("wolai base_url = %q, want %q", settings.BaseURL, "https://openapi.wolai.com")
		}
		return &stubWolaiClient{
			getBlockFunc: func(id string) (map[string]any, error) {
				testedBlockID = id
				return map[string]any{"id": id, "type": "page"}, nil
			},
		}, nil
	}

	updated, err := svc.UpdateWolaiSettings(model.WolaiSettings{
		Token:         " wolai-token ",
		ParentBlockID: " block-123 ",
		BaseURL:       "https://openapi.wolai.com/",
	})
	if err != nil {
		t.Fatalf("UpdateWolaiSettings() error = %v", err)
	}
	if updated.Token != "wolai-token" || updated.ParentBlockID != "block-123" || updated.BaseURL != "https://openapi.wolai.com" {
		t.Fatalf("UpdateWolaiSettings() = %+v, want normalized settings", updated)
	}

	reloaded, err := svc.GetWolaiSettings()
	if err != nil {
		t.Fatalf("GetWolaiSettings() error = %v", err)
	}
	if *reloaded != *updated {
		t.Fatalf("GetWolaiSettings() = %+v, want %+v", reloaded, updated)
	}

	result, err := svc.TestWolaiSettings(*reloaded)
	if err != nil {
		t.Fatalf("TestWolaiSettings() error = %v", err)
	}
	if !result.Success {
		t.Fatalf("TestWolaiSettings() success = %v, want true", result.Success)
	}
	if testedBlockID != "block-123" {
		t.Fatalf("TestWolaiSettings() tested block = %q, want %q", testedBlockID, "block-123")
	}

	raw, err := repo.GetAppSetting(wolaiSettingsKey)
	if err != nil {
		t.Fatalf("GetAppSetting(%q) error = %v", wolaiSettingsKey, err)
	}
	if !strings.Contains(raw, `"token":"wolai-token"`) || !strings.Contains(raw, `"parent_block_id":"block-123"`) {
		t.Fatalf("saved wolai settings = %q, want token and parent_block_id persisted", raw)
	}
}

func TestSavePaperNoteToWolaiBuildsStructuredBlocks(t *testing.T) {
	svc, repo, _ := newTestService(t)
	paper := createTestPaper(t, repo)

	if _, err := svc.UpdateWolaiSettings(model.WolaiSettings{
		Token:         "wolai-token",
		ParentBlockID: "paper-root",
	}); err != nil {
		t.Fatalf("UpdateWolaiSettings() error = %v", err)
	}

	type createCall struct {
		parentID string
		blocks   []map[string]any
	}

	var calls []createCall
	svc.wolaiClientFactory = func(settings model.WolaiSettings) (wolaiClient, error) {
		return &stubWolaiClient{
			createBlocksFunc: func(parentID string, blocks any) ([]wolaiapi.CreatedBlock, error) {
				typed, ok := blocks.([]map[string]any)
				if !ok {
					t.Fatalf("blocks type = %T, want []map[string]any", blocks)
				}
				calls = append(calls, createCall{parentID: parentID, blocks: typed})
				if len(calls) == 1 {
					return []wolaiapi.CreatedBlock{{ID: "paper-note-page", Type: "page"}}, nil
				}
				return []wolaiapi.CreatedBlock{{ID: "paper-note-body"}}, nil
			},
		}, nil
	}

	result, err := svc.SavePaperNoteToWolai(paper.ID, "## 结论\n\n这个结果支持免疫重编程。")
	if err != nil {
		t.Fatalf("SavePaperNoteToWolai() error = %v", err)
	}
	if !result.Success || result.TargetBlockID != "paper-note-page" {
		t.Fatalf("SavePaperNoteToWolai() result = %+v, want success on paper-note-page", result)
	}
	if len(calls) != 2 {
		t.Fatalf("CreateBlocks() calls = %d, want 2", len(calls))
	}
	if calls[0].parentID != "paper-root" {
		t.Fatalf("page CreateBlocks() parent_id = %q, want %q", calls[0].parentID, "paper-root")
	}
	if len(calls[0].blocks) != 1 || calls[0].blocks[0]["type"] != "page" || calls[0].blocks[0]["content"] != "文献笔记｜Atlas Study" {
		t.Fatalf("page CreateBlocks() blocks = %#v, want single page block", calls[0].blocks)
	}
	if calls[1].parentID != "paper-note-page" {
		t.Fatalf("body CreateBlocks() parent_id = %q, want %q", calls[1].parentID, "paper-note-page")
	}
	if len(calls[1].blocks) < 2 {
		t.Fatalf("body CreateBlocks() blocks = %#v, want at least 2 blocks", calls[1].blocks)
	}

	text := strings.Join(wolaiBlockContents(calls[1].blocks), "\n")
	types := wolaiBlockTypes(calls[1].blocks)
	if strings.Contains(text, "文献笔记｜Atlas Study") {
		t.Fatalf("saved text = %q, want page title stored only in page block", text)
	}
	if !strings.Contains(text, "原始文件：atlas-study.pdf") {
		t.Fatalf("saved text = %q, want original filename included", text)
	}
	if !strings.Contains(text, "当前分组：未分组") {
		t.Fatalf("saved text = %q, want default group included", text)
	}
	if !strings.Contains(text, "文献标签：Atlas") {
		t.Fatalf("saved text = %q, want tag metadata included", text)
	}
	if !strings.Contains(text, "Atlas abstract") {
		t.Fatalf("saved text = %q, want abstract included", text)
	}
	if !containsString(types, "heading") {
		t.Fatalf("body block types = %#v, want heading blocks for section styles", types)
	}
	if strings.Contains(text, "## 结论") {
		t.Fatalf("saved text = %q, want markdown heading converted to Wolai heading block", text)
	}
	foundConclusionHeading := false
	for _, block := range calls[1].blocks {
		if block["type"] == "heading" && wolaiBlockTitle(block) == "结论" {
			foundConclusionHeading = true
			break
		}
	}
	if !foundConclusionHeading {
		t.Fatalf("body CreateBlocks() blocks = %#v, want markdown heading converted", calls[1].blocks)
	}
	if !strings.Contains(text, "这个结果支持免疫重编程") {
		t.Fatalf("saved text = %q, want note body included", text)
	}
}

func TestSaveFigureNoteToWolaiUsesFigureMetadata(t *testing.T) {
	svc, repo, _ := newTestService(t)
	paper := createTestPaper(t, repo)

	if _, err := svc.UpdateWolaiSettings(model.WolaiSettings{
		Token:         "wolai-token",
		ParentBlockID: "figure-root",
	}); err != nil {
		t.Fatalf("UpdateWolaiSettings() error = %v", err)
	}

	type createCall struct {
		parentID string
		blocks   []map[string]any
	}

	var calls []createCall
	svc.wolaiClientFactory = func(settings model.WolaiSettings) (wolaiClient, error) {
		return &stubWolaiClient{
			createBlocksFunc: func(parentID string, blocks any) ([]wolaiapi.CreatedBlock, error) {
				typed, ok := blocks.([]map[string]any)
				if !ok {
					t.Fatalf("blocks type = %T, want []map[string]any", blocks)
				}
				calls = append(calls, createCall{parentID: parentID, blocks: typed})
				if len(calls) == 1 {
					return []wolaiapi.CreatedBlock{{ID: "figure-note-page", Type: "page"}}, nil
				}
				return []wolaiapi.CreatedBlock{{ID: "figure-note-body"}}, nil
			},
		}, nil
	}

	result, err := svc.SaveFigureNoteToWolai(paper.Figures[0].ID, "观察到信号增强。")
	if err != nil {
		t.Fatalf("SaveFigureNoteToWolai() error = %v", err)
	}
	if !result.Success || result.TargetBlockID != "figure-note-page" {
		t.Fatalf("SaveFigureNoteToWolai() result = %+v, want success on figure-note-page", result)
	}
	if len(calls) != 2 {
		t.Fatalf("CreateBlocks() calls = %d, want 2", len(calls))
	}
	if calls[0].parentID != "figure-root" {
		t.Fatalf("page CreateBlocks() parent_id = %q, want %q", calls[0].parentID, "figure-root")
	}
	if len(calls[0].blocks) != 1 || calls[0].blocks[0]["type"] != "page" || calls[0].blocks[0]["content"] != "图片笔记｜Atlas Study" {
		t.Fatalf("page CreateBlocks() blocks = %#v, want single page block", calls[0].blocks)
	}
	if calls[1].parentID != "figure-note-page" {
		t.Fatalf("body CreateBlocks() parent_id = %q, want %q", calls[1].parentID, "figure-note-page")
	}

	text := strings.Join(wolaiBlockContents(calls[1].blocks), "\n")
	if strings.Contains(text, "图片笔记｜Atlas Study") {
		t.Fatalf("saved text = %q, want page title stored only in page block", text)
	}
	if !strings.Contains(text, "来源文献：Atlas Study") {
		t.Fatalf("saved text = %q, want paper title metadata", text)
	}
	if !strings.Contains(text, "第 1 页") || !strings.Contains(text, "Fig 1") {
		t.Fatalf("saved text = %q, want figure location metadata", text)
	}
	if !strings.Contains(text, "Figure") {
		t.Fatalf("saved text = %q, want caption included", text)
	}
	if !strings.Contains(text, "观察到信号增强") {
		t.Fatalf("saved text = %q, want note body included", text)
	}
}

func TestSavePaperNoteToWolaiBatchesBlocksForWolaiLimit(t *testing.T) {
	svc, repo, _ := newTestService(t)
	paper := createTestPaper(t, repo)

	if _, err := svc.UpdateWolaiSettings(model.WolaiSettings{
		Token:         "wolai-token",
		ParentBlockID: "paper-root",
	}); err != nil {
		t.Fatalf("UpdateWolaiSettings() error = %v", err)
	}

	type createCall struct {
		parentID string
		blocks   []map[string]any
	}

	var calls []createCall
	svc.wolaiClientFactory = func(settings model.WolaiSettings) (wolaiClient, error) {
		return &stubWolaiClient{
			createBlocksFunc: func(parentID string, blocks any) ([]wolaiapi.CreatedBlock, error) {
				typed, ok := blocks.([]map[string]any)
				if !ok {
					t.Fatalf("blocks type = %T, want []map[string]any", blocks)
				}
				calls = append(calls, createCall{parentID: parentID, blocks: typed})
				if len(calls) == 1 {
					return []wolaiapi.CreatedBlock{{ID: "paper-note-page", Type: "page"}}, nil
				}
				return []wolaiapi.CreatedBlock{{ID: fmt.Sprintf("paper-note-body-%d", len(calls)-1)}}, nil
			},
		}, nil
	}

	parts := make([]string, 0, 12)
	for i := 1; i <= 12; i++ {
		parts = append(parts, fmt.Sprintf("## 部分 %d\n\n内容 %d", i, i))
	}
	notes := strings.Join(parts, "\n\n")

	result, err := svc.SavePaperNoteToWolai(paper.ID, notes)
	if err != nil {
		t.Fatalf("SavePaperNoteToWolai() error = %v", err)
	}
	if !result.Success || result.TargetBlockID != "paper-note-page" {
		t.Fatalf("SavePaperNoteToWolai() result = %+v, want success on paper-note-page", result)
	}
	if len(calls) < 3 {
		t.Fatalf("CreateBlocks() calls = %d, want page call plus at least 2 body batches", len(calls))
	}

	totalBodyBlocks := 0
	for i, call := range calls[1:] {
		if call.parentID != "paper-note-page" {
			t.Fatalf("body CreateBlocks() call %d parent_id = %q, want %q", i+1, call.parentID, "paper-note-page")
		}
		if len(call.blocks) == 0 || len(call.blocks) > wolaiCreateBlocksBatchSize {
			t.Fatalf("body CreateBlocks() call %d block count = %d, want 1..%d", i+1, len(call.blocks), wolaiCreateBlocksBatchSize)
		}
		totalBodyBlocks += len(call.blocks)
	}
	if totalBodyBlocks <= wolaiCreateBlocksBatchSize {
		t.Fatalf("total body blocks = %d, want more than Wolai batch size %d", totalBodyBlocks, wolaiCreateBlocksBatchSize)
	}
}

func TestBuildWolaiMarkdownBlocksUsesBlockStyles(t *testing.T) {
	blocks := buildWolaiMarkdownBlocks(strings.Join([]string{
		"# 一级标题",
		"",
		"- 无序项",
		"- [x] 带勾选标记",
		"",
		"1. 第一项",
		"2. 第二项",
		"",
		"> 引用说明",
		"> 第二行",
		"",
		"```go",
		"fmt.Println(\"hi\")",
		"```",
		"",
		"---",
		"",
		"普通段落",
	}, "\n"))

	if got := wolaiBlockTypes(blocks); !reflect.DeepEqual(got, []string{
		"heading",
		"bull_list",
		"bull_list",
		"enum_list",
		"enum_list",
		"quote",
		"code",
		"divider",
		"text",
	}) {
		t.Fatalf("buildWolaiMarkdownBlocks() types = %#v", got)
	}

	if wolaiBlockTitle(blocks[0]) != "一级标题" || blocks[0]["level"] != 1 {
		t.Fatalf("heading block = %#v, want level 1 heading", blocks[0])
	}
	if _, ok := blocks[0]["content"].(map[string]any); !ok {
		t.Fatalf("heading block content = %#v, want object with title", blocks[0]["content"])
	}
	if blocks[1]["content"] != "无序项" {
		t.Fatalf("bullet block = %#v, want stripped bullet marker", blocks[1])
	}
	if blocks[2]["content"] != "[x] 带勾选标记" {
		t.Fatalf("bullet block = %#v, want checklist text preserved", blocks[2])
	}
	if blocks[5]["content"] != "引用说明\n第二行" {
		t.Fatalf("quote block = %#v, want merged quote lines", blocks[5])
	}
	if blocks[6]["language"] != "go" || blocks[6]["content"] != "fmt.Println(\"hi\")" {
		t.Fatalf("code block = %#v, want code language and content", blocks[6])
	}
	if blocks[8]["content"] != "普通段落" {
		t.Fatalf("text block = %#v, want plain paragraph", blocks[8])
	}
}

func TestSavePaperNoteToWolaiReplacesMarkdownImagesWithTODO(t *testing.T) {
	svc, repo, _ := newTestService(t)
	paper := createTestPaper(t, repo)

	if _, err := svc.UpdateWolaiSettings(model.WolaiSettings{
		Token:         "wolai-token",
		ParentBlockID: "paper-root",
	}); err != nil {
		t.Fatalf("UpdateWolaiSettings() error = %v", err)
	}

	var blocksByCall [][]map[string]any
	svc.wolaiClientFactory = func(settings model.WolaiSettings) (wolaiClient, error) {
		return &stubWolaiClient{
			createBlocksFunc: func(parentID string, blocks any) ([]wolaiapi.CreatedBlock, error) {
				typed, ok := blocks.([]map[string]any)
				if !ok {
					t.Fatalf("blocks type = %T, want []map[string]any", blocks)
				}
				blocksByCall = append(blocksByCall, typed)
				if len(blocksByCall) == 1 {
					return []wolaiapi.CreatedBlock{{ID: "paper-note-page", Type: "page"}}, nil
				}
				return []wolaiapi.CreatedBlock{{ID: "paper-note-body"}}, nil
			},
		}, nil
	}

	result, err := svc.SavePaperNoteToWolai(paper.ID, fmt.Sprintf("结论如下：\n\n![第 1 页图 1](figure://%d)\n\n继续分析。", paper.Figures[0].ID))
	if err != nil {
		t.Fatalf("SavePaperNoteToWolai() error = %v", err)
	}
	if !strings.Contains(result.Message, "笔记内图片已标记 TODO") {
		t.Fatalf("SavePaperNoteToWolai() message = %q, want TODO warning", result.Message)
	}
	if len(blocksByCall) != 2 {
		t.Fatalf("CreateBlocks() calls = %d, want 2", len(blocksByCall))
	}

	text := strings.Join(wolaiBlockContents(blocksByCall[1]), "\n")
	if strings.Contains(text, fmt.Sprintf("figure://%d", paper.Figures[0].ID)) {
		t.Fatalf("saved text = %q, want markdown image source removed", text)
	}
	if !strings.Contains(text, "【TODO：图片“第 1 页图 1”暂不支持保存到 Wolai，等待后续完成。】") {
		t.Fatalf("saved text = %q, want image TODO placeholder", text)
	}
}

func TestSaveFigureNoteToWolaiReplacesMarkdownImagesWithTODO(t *testing.T) {
	svc, repo, _ := newTestService(t)
	paper := createTestPaper(t, repo)

	if _, err := svc.UpdateWolaiSettings(model.WolaiSettings{
		Token:         "wolai-token",
		ParentBlockID: "figure-root",
	}); err != nil {
		t.Fatalf("UpdateWolaiSettings() error = %v", err)
	}

	var blocksByCall [][]map[string]any
	svc.wolaiClientFactory = func(settings model.WolaiSettings) (wolaiClient, error) {
		return &stubWolaiClient{
			createBlocksFunc: func(parentID string, blocks any) ([]wolaiapi.CreatedBlock, error) {
				typed, ok := blocks.([]map[string]any)
				if !ok {
					t.Fatalf("blocks type = %T, want []map[string]any", blocks)
				}
				blocksByCall = append(blocksByCall, typed)
				if len(blocksByCall) == 1 {
					return []wolaiapi.CreatedBlock{{ID: "figure-note-page", Type: "page"}}, nil
				}
				return []wolaiapi.CreatedBlock{{ID: "figure-note-body"}}, nil
			},
		}, nil
	}

	result, err := svc.SaveFigureNoteToWolai(paper.Figures[0].ID, "请补看：![局部放大图](https://example.com/figure.png)")
	if err != nil {
		t.Fatalf("SaveFigureNoteToWolai() error = %v", err)
	}
	if !strings.Contains(result.Message, "笔记内图片已标记 TODO") {
		t.Fatalf("SaveFigureNoteToWolai() message = %q, want TODO warning", result.Message)
	}
	if len(blocksByCall) != 2 {
		t.Fatalf("CreateBlocks() calls = %d, want 2", len(blocksByCall))
	}

	text := strings.Join(wolaiBlockContents(blocksByCall[1]), "\n")
	if strings.Contains(text, "https://example.com/figure.png") {
		t.Fatalf("saved text = %q, want external image source removed", text)
	}
	if !strings.Contains(text, "【TODO：图片“局部放大图”暂不支持保存到 Wolai，等待后续完成。】") {
		t.Fatalf("saved text = %q, want image TODO placeholder", text)
	}
}

func TestInsertWolaiTestPageCreatesChildPageAndWritesImageTODO(t *testing.T) {
	svc, _, _ := newTestService(t)

	type createCall struct {
		parentID string
		blocks   []map[string]any
	}

	var calls []createCall

	svc.wolaiClientFactory = func(settings model.WolaiSettings) (wolaiClient, error) {
		return &stubWolaiClient{
			createBlocksFunc: func(parentID string, blocks any) ([]wolaiapi.CreatedBlock, error) {
				typed, ok := blocks.([]map[string]any)
				if !ok {
					t.Fatalf("blocks type = %T, want []map[string]any", blocks)
				}
				calls = append(calls, createCall{parentID: parentID, blocks: typed})

				switch len(calls) {
				case 1:
					return []wolaiapi.CreatedBlock{{ID: "test-page-1", Type: "page", URL: "https://www.wolai.com/workspace/test-page-1"}}, nil
				case 2:
					return []wolaiapi.CreatedBlock{{ID: "text-block-1", Type: "text"}}, nil
				case 3:
					return []wolaiapi.CreatedBlock{{ID: "todo-block-1", Type: "text"}}, nil
				default:
					t.Fatalf("unexpected CreateBlocks() call #%d", len(calls))
					return nil, nil
				}
			},
		}, nil
	}

	result, err := svc.InsertWolaiTestPage(model.WolaiSettings{
		Token:         "wolai-token",
		ParentBlockID: "root-page",
	})
	if err != nil {
		t.Fatalf("InsertWolaiTestPage() error = %v", err)
	}

	if !result.Success || result.TargetBlockID != "test-page-1" {
		t.Fatalf("InsertWolaiTestPage() result = %+v, want success on test-page-1", result)
	}
	if result.TargetBlockURL != "https://www.wolai.com/workspace/test-page-1" {
		t.Fatalf("InsertWolaiTestPage() target_block_url = %q, want Wolai page URL", result.TargetBlockURL)
	}
	if len(calls) != 3 {
		t.Fatalf("CreateBlocks() calls = %d, want 3", len(calls))
	}
	if calls[0].parentID != "root-page" || len(calls[0].blocks) != 1 || calls[0].blocks[0]["type"] != "page" || calls[0].blocks[0]["content"] != "Test Page" {
		t.Fatalf("page CreateBlocks() = %#v", calls[0])
	}
	if calls[1].parentID != "test-page-1" || calls[1].blocks[0]["content"] != "Test works" {
		t.Fatalf("text CreateBlocks() = %#v", calls[1])
	}
	if !strings.Contains(result.Message, "图片导出 TODO") {
		t.Fatalf("InsertWolaiTestPage() message = %q, want TODO notice", result.Message)
	}
	if calls[2].parentID != "test-page-1" || len(calls[2].blocks) != 1 || calls[2].blocks[0]["type"] != "text" {
		t.Fatalf("todo CreateBlocks() = %#v", calls[2])
	}
	if calls[2].blocks[0]["content"] != wolaiTestPageImageTODOText {
		t.Fatalf("todo CreateBlocks() content = %#v, want image TODO text", calls[2].blocks[0])
	}
}

func TestDeletePaperRemovesFilesAndReturnsNotFound(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	paper := createTestPaper(t, repo)

	if err := os.WriteFile(filepath.Join(cfg.PapersDir(), paper.StoredPDFName), []byte("pdf"), 0o644); err != nil {
		t.Fatalf("WriteFile(pdf) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename), []byte("img"), 0o644); err != nil {
		t.Fatalf("WriteFile(figure) error = %v", err)
	}

	if err := svc.DeletePaper(paper.ID); err != nil {
		t.Fatalf("DeletePaper() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(cfg.PapersDir(), paper.StoredPDFName)); !os.IsNotExist(err) {
		t.Fatalf("paper file still exists, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename)); !os.IsNotExist(err) {
		t.Fatalf("figure file still exists, stat err = %v", err)
	}

	if err := svc.DeletePaper(paper.ID); !apperr.IsCode(err, apperr.CodeNotFound) {
		t.Fatalf("DeletePaper() missing code = %q, want %q", apperr.CodeOf(err), apperr.CodeNotFound)
	}
}

func TestUpdatePaperValidationErrors(t *testing.T) {
	svc, _, _ := newTestService(t)

	if _, err := svc.UpdatePaper(1, UpdatePaperParams{Title: "   "}); !apperr.IsCode(err, apperr.CodeInvalidArgument) {
		t.Fatalf("UpdatePaper() empty title code = %q, want %q", apperr.CodeOf(err), apperr.CodeInvalidArgument)
	}

	groupID := int64(999)
	if _, err := svc.UpdatePaper(1, UpdatePaperParams{Title: "Valid", GroupID: &groupID}); !apperr.IsCode(err, apperr.CodeNotFound) {
		t.Fatalf("UpdatePaper() missing group code = %q, want %q", apperr.CodeOf(err), apperr.CodeNotFound)
	}
}

func TestUpdatePaperPersistsMetadata(t *testing.T) {
	svc, repo, _ := newTestService(t)
	paper := createTestPaper(t, repo)
	nextPDFText := "Updated full text"

	updated, err := svc.UpdatePaper(paper.ID, UpdatePaperParams{
		Title:          "Atlas Study Revised",
		PDFText:        &nextPDFText,
		AbstractText:   "Updated abstract",
		NotesText:      "Updated notes",
		PaperNotesText: "Updated paper notes",
		Tags:           []string{"Atlas", "Revised"},
	})
	if err != nil {
		t.Fatalf("UpdatePaper() error = %v", err)
	}

	if updated.AbstractText != "Updated abstract" || updated.NotesText != "Updated notes" || updated.PaperNotesText != "Updated paper notes" {
		t.Fatalf("UpdatePaper() metadata = (%q, %q, %q), want updated values", updated.AbstractText, updated.NotesText, updated.PaperNotesText)
	}
	if updated.PDFText != nextPDFText {
		t.Fatalf("UpdatePaper() pdf_text = %q, want %q", updated.PDFText, nextPDFText)
	}
	if len(updated.Tags) != 2 {
		t.Fatalf("UpdatePaper() tags = %d, want 2", len(updated.Tags))
	}
}

func TestUpdatePaperKeepsPDFTextWhenOmitted(t *testing.T) {
	svc, repo, _ := newTestService(t)
	paper := createTestPaper(t, repo)

	updated, err := svc.UpdatePaper(paper.ID, UpdatePaperParams{
		Title:        "Atlas Study Retitled",
		AbstractText: "Fresh abstract",
	})
	if err != nil {
		t.Fatalf("UpdatePaper() error = %v", err)
	}

	if updated.PDFText != "Atlas full text" {
		t.Fatalf("UpdatePaper() pdf_text = %q, want %q", updated.PDFText, "Atlas full text")
	}
}

func TestUpdatePaperPDFTextOnlyPersistsText(t *testing.T) {
	svc, repo, _ := newTestService(t)
	paper := createTestPaper(t, repo)

	updated, err := svc.UpdatePaperPDFText(paper.ID, "  Extracted PDF full text  ")
	if err != nil {
		t.Fatalf("UpdatePaperPDFText() error = %v", err)
	}

	if updated.PDFText != "Extracted PDF full text" {
		t.Fatalf("UpdatePaperPDFText() pdf_text = %q, want %q", updated.PDFText, "Extracted PDF full text")
	}
	if updated.Title != paper.Title || updated.AbstractText != paper.AbstractText || updated.NotesText != paper.NotesText {
		t.Fatalf("UpdatePaperPDFText() mutated metadata: %+v", updated)
	}
}

func TestUpdatePaperPDFTextRejectsEmpty(t *testing.T) {
	svc, _, _ := newTestService(t)

	if _, err := svc.UpdatePaperPDFText(1, "   "); !apperr.IsCode(err, apperr.CodeInvalidArgument) {
		t.Fatalf("UpdatePaperPDFText() code = %q, want %q", apperr.CodeOf(err), apperr.CodeInvalidArgument)
	}
}

func TestUpdateFigureTagsOnlyTouchesSelectedFigure(t *testing.T) {
	svc, repo, _ := newTestService(t)

	paper, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Figure Tags",
		OriginalFilename: "figure-tags.pdf",
		StoredPDFName:    "figure-tags.pdf",
		FileSize:         256,
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
		Figures: []repository.FigureUpsertInput{
			{Filename: "figure_a.png", ContentType: "image/png", PageNumber: 1, FigureIndex: 1, Caption: "A"},
			{Filename: "figure_b.png", ContentType: "image/png", PageNumber: 1, FigureIndex: 2, Caption: "B"},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}

	updated, err := svc.UpdateFigureTags(paper.Figures[0].ID, []string{"signal"})
	if err != nil {
		t.Fatalf("UpdateFigureTags() error = %v", err)
	}

	if got := len(updated.Figures[0].Tags); got != 1 {
		t.Fatalf("updated first figure tags = %d, want 1", got)
	}
	if got := len(updated.Figures[1].Tags); got != 0 {
		t.Fatalf("updated second figure tags = %d, want 0", got)
	}
	if got := updated.Figures[0].ImageURL; got != "/files/figures/figure_a.png" {
		t.Fatalf("updated first figure image_url = %q, want %q", got, "/files/figures/figure_a.png")
	}

	tagID := updated.Figures[0].Tags[0].ID
	result, err := svc.ListFigures(model.FigureFilter{TagID: &tagID})
	if err != nil {
		t.Fatalf("ListFigures() error = %v", err)
	}
	if result.Total != 1 || len(result.Figures) != 1 {
		t.Fatalf("ListFigures() total=%d len=%d, want 1/1", result.Total, len(result.Figures))
	}
	if result.Figures[0].ID != paper.Figures[0].ID {
		t.Fatalf("ListFigures() figure id = %d, want %d", result.Figures[0].ID, paper.Figures[0].ID)
	}
}

func TestUpdateFigureNotesAreSearchable(t *testing.T) {
	svc, repo, _ := newTestService(t)

	paper, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Figure Notes",
		OriginalFilename: "figure-notes.pdf",
		StoredPDFName:    "figure-notes.pdf",
		FileSize:         256,
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
		Figures: []repository.FigureUpsertInput{
			{Filename: "figure_a.png", ContentType: "image/png", PageNumber: 1, FigureIndex: 1, Caption: "A"},
			{Filename: "figure_b.png", ContentType: "image/png", PageNumber: 1, FigureIndex: 2, Caption: "B"},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}

	notes := "AI 总结：该图片强调了微环境重塑。"
	updated, err := svc.UpdateFigure(paper.Figures[0].ID, UpdateFigureParams{NotesText: &notes})
	if err != nil {
		t.Fatalf("UpdateFigure() error = %v", err)
	}

	if updated.Figures[0].NotesText != notes {
		t.Fatalf("updated first figure notes_text = %q, want %q", updated.Figures[0].NotesText, notes)
	}
	if updated.Figures[0].Caption != "A" {
		t.Fatalf("updated first figure caption = %q, want %q", updated.Figures[0].Caption, "A")
	}
	if updated.Figures[1].NotesText != "" {
		t.Fatalf("updated second figure notes_text = %q, want empty", updated.Figures[1].NotesText)
	}
	if updated.Figures[1].Caption != "B" {
		t.Fatalf("updated second figure caption = %q, want %q", updated.Figures[1].Caption, "B")
	}

	result, err := svc.ListFigures(model.FigureFilter{Keyword: "微环境重塑"})
	if err != nil {
		t.Fatalf("ListFigures() error = %v", err)
	}
	if result.Total != 1 || len(result.Figures) != 1 {
		t.Fatalf("ListFigures() total=%d len=%d, want 1/1", result.Total, len(result.Figures))
	}
	if result.Figures[0].ID != paper.Figures[0].ID {
		t.Fatalf("ListFigures() figure id = %d, want %d", result.Figures[0].ID, paper.Figures[0].ID)
	}
	if result.Figures[0].NotesText != notes {
		t.Fatalf("ListFigures() notes_text = %q, want %q", result.Figures[0].NotesText, notes)
	}
}

func TestPurgeLibraryRemovesStoredAssets(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	paper := createTestPaper(t, repo)

	if err := os.WriteFile(filepath.Join(cfg.PapersDir(), paper.StoredPDFName), []byte("pdf"), 0o644); err != nil {
		t.Fatalf("WriteFile(pdf) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename), []byte("img"), 0o644); err != nil {
		t.Fatalf("WriteFile(figure) error = %v", err)
	}

	if err := svc.PurgeLibrary(); err != nil {
		t.Fatalf("PurgeLibrary() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(cfg.PapersDir(), paper.StoredPDFName)); !os.IsNotExist(err) {
		t.Fatalf("paper file still exists, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename)); !os.IsNotExist(err) {
		t.Fatalf("figure file still exists, stat err = %v", err)
	}

	result, err := svc.ListPapers(model.PaperFilter{})
	if err != nil {
		t.Fatalf("ListPapers() error = %v", err)
	}
	if result.Total != 0 || len(result.Papers) != 0 {
		t.Fatalf("ListPapers() after purge = total:%d len:%d", result.Total, len(result.Papers))
	}
}

func TestExtractorSettingsDefaultsAndPersistence(t *testing.T) {
	svc, _, cfg := newTestService(t)

	defaults, err := svc.GetExtractorSettings()
	if err != nil {
		t.Fatalf("GetExtractorSettings() default error = %v", err)
	}
	if defaults.ExtractorFileField != "file" || defaults.TimeoutSeconds != cfg.ExtractorTimeoutSeconds {
		t.Fatalf("GetExtractorSettings() defaults = %+v, want config defaults", defaults)
	}
	if defaults.ExtractorProfile != extractorProfilePDFFigXV1 {
		t.Fatalf("GetExtractorSettings() extractor_profile = %q, want %q", defaults.ExtractorProfile, extractorProfilePDFFigXV1)
	}
	if defaults.PDFTextSource != pdfTextSourceExtractor {
		t.Fatalf("GetExtractorSettings() pdf_text_source = %q, want %q", defaults.PDFTextSource, pdfTextSourceExtractor)
	}

	updated, err := svc.UpdateExtractorSettings(model.ExtractorSettings{
		ExtractorProfile:    extractorProfileOpenSourceVision,
		PDFTextSource:       pdfTextSourcePDFJS,
		ExtractorURL:        "http://127.0.0.1:9000/api/v1/extract",
		ExtractorToken:      "secret",
		ExtractorFileField:  "upload",
		TimeoutSeconds:      120,
		PollIntervalSeconds: 5,
	})
	if err != nil {
		t.Fatalf("UpdateExtractorSettings() error = %v", err)
	}
	if updated.EffectiveExtractorURL == "" || updated.EffectiveJobsURL == "" || updated.ExtractorFileField != "upload" {
		t.Fatalf("UpdateExtractorSettings() = %+v, want normalized effective values", updated)
	}
	if updated.ExtractorProfile != extractorProfileOpenSourceVision || updated.PDFTextSource != pdfTextSourcePDFJS {
		t.Fatalf("UpdateExtractorSettings() profile/text_source = (%q,%q), want (%q,%q)", updated.ExtractorProfile, updated.PDFTextSource, extractorProfileOpenSourceVision, pdfTextSourcePDFJS)
	}
	if updated.ExtractorJobsURL != "" {
		t.Fatalf("UpdateExtractorSettings() extractor_jobs_url = %q, want empty", updated.ExtractorJobsURL)
	}
}

func TestBuiltInLLMExtractorForcesPDFJSTextSource(t *testing.T) {
	svc, _, _ := newTestService(t)

	updated, err := svc.UpdateExtractorSettings(model.ExtractorSettings{
		ExtractorProfile: extractorProfileOpenSourceVision,
		PDFTextSource:    pdfTextSourceExtractor,
	})
	if err != nil {
		t.Fatalf("UpdateExtractorSettings() error = %v", err)
	}
	if updated.PDFTextSource != pdfTextSourcePDFJS {
		t.Fatalf("UpdateExtractorSettings() pdf_text_source = %q, want %q", updated.PDFTextSource, pdfTextSourcePDFJS)
	}
}

func TestManualExtractorForcesPDFJSTextSource(t *testing.T) {
	svc, _, _ := newTestService(t)

	updated, err := svc.UpdateExtractorSettings(model.ExtractorSettings{
		ExtractorProfile: extractorProfileManual,
		PDFTextSource:    pdfTextSourceExtractor,
	})
	if err != nil {
		t.Fatalf("UpdateExtractorSettings() error = %v", err)
	}
	if updated.PDFTextSource != pdfTextSourcePDFJS {
		t.Fatalf("UpdateExtractorSettings() pdf_text_source = %q, want %q", updated.PDFTextSource, pdfTextSourcePDFJS)
	}
}

func TestPDFFigXExtractorForcesExtractorTextSource(t *testing.T) {
	svc, _, _ := newTestService(t)

	updated, err := svc.UpdateExtractorSettings(model.ExtractorSettings{
		ExtractorProfile: extractorProfilePDFFigXV1,
		PDFTextSource:    pdfTextSourcePDFJS,
	})
	if err != nil {
		t.Fatalf("UpdateExtractorSettings() error = %v", err)
	}
	if updated.PDFTextSource != pdfTextSourceExtractor {
		t.Fatalf("UpdateExtractorSettings() pdf_text_source = %q, want %q", updated.PDFTextSource, pdfTextSourceExtractor)
	}
}

func TestBuildExtractorUploadBodyUsesRuntimeFileField(t *testing.T) {
	svc, _, cfg := newTestService(t)

	pdfPath := filepath.Join(cfg.PapersDir(), "sample.pdf")
	if err := os.MkdirAll(filepath.Dir(pdfPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.4 test"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	body, _, err := svc.buildExtractorUploadBody(model.ExtractorSettings{
		ExtractorFileField: "upload",
	}, pdfPath, "sample.pdf")
	if err != nil {
		t.Fatalf("buildExtractorUploadBody() error = %v", err)
	}

	if !bytes.Contains(body.Bytes(), []byte(`name="upload"`)) {
		t.Fatalf("buildExtractorUploadBody() body missing configured file field: %s", body.String())
	}
	if !bytes.Contains(body.Bytes(), []byte("name=\"include_pdf_text\"")) || !bytes.Contains(body.Bytes(), []byte("\r\n\r\ntrue")) {
		t.Fatalf("buildExtractorUploadBody() body missing include_pdf_text=true: %s", body.String())
	}
}

func TestBuildExtractorUploadBodyDisablesPDFTextWhenUsingPDFJS(t *testing.T) {
	svc, _, cfg := newTestService(t)

	pdfPath := filepath.Join(cfg.PapersDir(), "sample-pdfjs.pdf")
	if err := os.MkdirAll(filepath.Dir(pdfPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.4 test"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	body, _, err := svc.buildExtractorUploadBody(model.ExtractorSettings{
		ExtractorProfile:   extractorProfileOpenSourceVision,
		PDFTextSource:      pdfTextSourcePDFJS,
		ExtractorFileField: "upload",
	}, pdfPath, "sample-pdfjs.pdf")
	if err != nil {
		t.Fatalf("buildExtractorUploadBody() error = %v", err)
	}

	if !bytes.Contains(body.Bytes(), []byte("name=\"include_pdf_text\"")) || !bytes.Contains(body.Bytes(), []byte("\r\n\r\nfalse")) {
		t.Fatalf("buildExtractorUploadBody() body missing include_pdf_text=false: %s", body.String())
	}
}

func TestUploadPaperWithoutExtractorConfiguredUsesCompleted(t *testing.T) {
	svc, _, _ := newTestService(t)

	content := []byte("%PDF-1.4 test")
	file := &testMultipartFile{Reader: bytes.NewReader(content)}
	header := &multipart.FileHeader{
		Filename: "manual-only.pdf",
		Size:     int64(len(content)),
		Header: textproto.MIMEHeader{
			"Content-Type": []string{"application/pdf"},
		},
	}

	paper, err := svc.UploadPaper(file, header, UploadPaperParams{Title: "Manual Only"})
	if err != nil {
		t.Fatalf("UploadPaper() error = %v", err)
	}

	if paper.ExtractionStatus != "completed" {
		t.Fatalf("UploadPaper() status = %q, want %q", paper.ExtractionStatus, "completed")
	}
	if !strings.Contains(paper.ExtractorMessage, "文献已入库") {
		t.Fatalf("UploadPaper() extractor_message = %q, want library-ready hint", paper.ExtractorMessage)
	}
}

func TestUploadPaperWithAutoModeRequiresConfiguredExtractor(t *testing.T) {
	svc, _, _ := newTestService(t)

	content := []byte("%PDF-1.4 test")
	file := &testMultipartFile{Reader: bytes.NewReader(content)}
	header := &multipart.FileHeader{
		Filename: "auto-mode.pdf",
		Size:     int64(len(content)),
		Header: textproto.MIMEHeader{
			"Content-Type": []string{"application/pdf"},
		},
	}

	_, err := svc.UploadPaper(file, header, UploadPaperParams{
		Title:          "Auto Mode",
		ExtractionMode: "auto",
	})
	if !apperr.IsCode(err, apperr.CodeFailedPrecondition) {
		t.Fatalf("UploadPaper() code = %q, want %q", apperr.CodeOf(err), apperr.CodeFailedPrecondition)
	}
}

func TestUploadPaperWithManualModeSkipsConfiguredExtractor(t *testing.T) {
	svc, _, _ := newTestService(t)

	if _, err := svc.UpdateExtractorSettings(model.ExtractorSettings{
		ExtractorURL:        "http://127.0.0.1:9000/api/v1/extract",
		ExtractorToken:      "secret",
		ExtractorFileField:  "upload",
		TimeoutSeconds:      120,
		PollIntervalSeconds: 5,
	}); err != nil {
		t.Fatalf("UpdateExtractorSettings() error = %v", err)
	}

	content := []byte("%PDF-1.4 test")
	file := &testMultipartFile{Reader: bytes.NewReader(content)}
	header := &multipart.FileHeader{
		Filename: "manual-mode.pdf",
		Size:     int64(len(content)),
		Header: textproto.MIMEHeader{
			"Content-Type": []string{"application/pdf"},
		},
	}

	paper, err := svc.UploadPaper(file, header, UploadPaperParams{
		Title:          "Manual Mode",
		ExtractionMode: "manual",
	})
	if err != nil {
		t.Fatalf("UploadPaper() error = %v", err)
	}

	if paper.ExtractionStatus != "completed" {
		t.Fatalf("UploadPaper() status = %q, want %q", paper.ExtractionStatus, "completed")
	}
	if !strings.Contains(paper.ExtractorMessage, "手工标注") {
		t.Fatalf("UploadPaper() extractor_message = %q, want manual hint", paper.ExtractorMessage)
	}
}

func TestUploadPaperWithManualModeBackfillsPDFTextInBackground(t *testing.T) {
	svc, _, _ := newTestService(t)
	svc.startBackground = true
	svc.pdfTextExtractor = func(path string) (string, error) {
		return "manual upload full text", nil
	}

	content := []byte("%PDF-1.4 manual fallback")
	file := &testMultipartFile{Reader: bytes.NewReader(content)}
	header := &multipart.FileHeader{
		Filename: "manual-fallback.pdf",
		Size:     int64(len(content)),
		Header: textproto.MIMEHeader{
			"Content-Type": []string{"application/pdf"},
		},
	}

	paper, err := svc.UploadPaper(file, header, UploadPaperParams{
		Title:          "Manual Fallback",
		ExtractionMode: "manual",
	})
	if err != nil {
		t.Fatalf("UploadPaper() error = %v", err)
	}

	if got := waitForPaperPDFText(t, svc, paper.ID); got != "manual upload full text" {
		t.Fatalf("waitForPaperPDFText() = %q, want %q", got, "manual upload full text")
	}
}

func TestUploadPaperWithManualExtractorProfileIgnoresConfiguredPDFFigX(t *testing.T) {
	svc, _, _ := newTestService(t)
	svc.startBackground = true
	svc.pdfTextExtractor = func(path string) (string, error) {
		return "manual profile full text", nil
	}

	if _, err := svc.UpdateExtractorSettings(model.ExtractorSettings{
		ExtractorProfile:    extractorProfileManual,
		ExtractorURL:        "http://127.0.0.1:9000/api/v1/extract",
		ExtractorToken:      "secret",
		ExtractorFileField:  "upload",
		TimeoutSeconds:      120,
		PollIntervalSeconds: 5,
	}); err != nil {
		t.Fatalf("UpdateExtractorSettings() error = %v", err)
	}

	content := []byte("%PDF-1.4 manual profile")
	file := &testMultipartFile{Reader: bytes.NewReader(content)}
	header := &multipart.FileHeader{
		Filename: "manual-profile.pdf",
		Size:     int64(len(content)),
		Header: textproto.MIMEHeader{
			"Content-Type": []string{"application/pdf"},
		},
	}

	paper, err := svc.UploadPaper(file, header, UploadPaperParams{Title: "Manual Profile"})
	if err != nil {
		t.Fatalf("UploadPaper() error = %v", err)
	}
	if paper.ExtractionStatus != "completed" {
		t.Fatalf("UploadPaper() status = %q, want %q", paper.ExtractionStatus, "completed")
	}
	if !strings.Contains(paper.ExtractorMessage, "当前 PDF 提取方案为手工") {
		t.Fatalf("UploadPaper() extractor_message = %q, want manual-profile hint", paper.ExtractorMessage)
	}
	if got := waitForPaperPDFText(t, svc, paper.ID); got != "manual profile full text" {
		t.Fatalf("waitForPaperPDFText() = %q, want %q", got, "manual profile full text")
	}
}

func TestUploadPaperRejectsAutoModeWhenManualExtractorProfileSelected(t *testing.T) {
	svc, _, _ := newTestService(t)

	if _, err := svc.UpdateExtractorSettings(model.ExtractorSettings{
		ExtractorProfile: extractorProfileManual,
	}); err != nil {
		t.Fatalf("UpdateExtractorSettings() error = %v", err)
	}

	content := []byte("%PDF-1.4 manual profile auto")
	file := &testMultipartFile{Reader: bytes.NewReader(content)}
	header := &multipart.FileHeader{
		Filename: "manual-profile-auto.pdf",
		Size:     int64(len(content)),
		Header: textproto.MIMEHeader{
			"Content-Type": []string{"application/pdf"},
		},
	}

	_, err := svc.UploadPaper(file, header, UploadPaperParams{
		Title:          "Manual Profile Auto",
		ExtractionMode: "auto",
	})
	if !apperr.IsCode(err, apperr.CodeFailedPrecondition) {
		t.Fatalf("UploadPaper() code = %q, want %q", apperr.CodeOf(err), apperr.CodeFailedPrecondition)
	}
	if !strings.Contains(err.Error(), "当前 PDF 提取方案为手工") {
		t.Fatalf("UploadPaper() error = %v, want manual-profile message", err)
	}
}

func TestUploadPaperWithBuiltInLLMAutoModeQueuesBackgroundTask(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	aiSvc := NewAIService(repo, cfg, nil)

	if _, err := svc.UpdateExtractorSettings(model.ExtractorSettings{
		ExtractorProfile: extractorProfileOpenSourceVision,
	}); err != nil {
		t.Fatalf("UpdateExtractorSettings() error = %v", err)
	}
	if _, err := aiSvc.UpdateSettings(model.AISettings{
		Models: []model.AIModelConfig{
			{
				ID:              "figure",
				Name:            "Figure",
				Provider:        model.AIProviderOpenAI,
				APIKey:          "test-key",
				BaseURL:         "https://api.openai.com",
				Model:           "gpt-test",
				MaxOutputTokens: 1200,
			},
		},
		SceneModels: model.AISceneModelSelection{
			DefaultModelID: "figure",
			FigureModelID:  "figure",
		},
		SystemPrompt: "system",
		FigurePrompt: "figure",
	}); err != nil {
		t.Fatalf("UpdateSettings() error = %v", err)
	}

	content := []byte("%PDF-1.4 built-in llm test")
	file := &testMultipartFile{Reader: bytes.NewReader(content)}
	header := &multipart.FileHeader{
		Filename: "llm-auto.pdf",
		Size:     int64(len(content)),
		Header: textproto.MIMEHeader{
			"Content-Type": []string{"application/pdf"},
		},
	}

	paper, err := svc.UploadPaper(file, header, UploadPaperParams{
		Title:          "Built-in LLM Auto",
		ExtractionMode: "auto",
	})
	if err != nil {
		t.Fatalf("UploadPaper() error = %v", err)
	}
	if paper.ExtractionStatus != "queued" {
		t.Fatalf("UploadPaper() status = %q, want %q", paper.ExtractionStatus, "queued")
	}
	if !strings.Contains(paper.ExtractorMessage, "等待内置 AI") {
		t.Fatalf("UploadPaper() extractor_message = %q, want built-in queue hint", paper.ExtractorMessage)
	}
}

func TestPersistExtractionResultMapsBuiltInLLMFiguresToAutoSource(t *testing.T) {
	svc, repo, _ := newTestService(t)

	paper, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "LLM Source Mapping",
		OriginalFilename: "llm-source.pdf",
		StoredPDFName:    "llm-source.pdf",
		FileSize:         256,
		ContentType:      "application/pdf",
		ExtractionStatus: "queued",
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}

	imageData := strings.TrimPrefix(testPNGDataURL(t, 24, 18), "data:image/png;base64,")
	if err := svc.persistExtractionResult(paper.ID, "", model.ExtractorSettings{
		ExtractorProfile: extractorProfileOpenSourceVision,
		PDFTextSource:    pdfTextSourcePDFJS,
	}, &extractionResult{
		PDFText: "full text",
		Boxes:   json.RawMessage(`[]`),
		Figures: []extractedFigure{
			{
				Filename:    "llm.png",
				ContentType: "image/png",
				PageNumber:  1,
				FigureIndex: 1,
				BBox:        json.RawMessage(`{"source":"llm"}`),
				Data:        imageData,
				Source:      manualFigureSourceLLM,
			},
		},
	}); err != nil {
		t.Fatalf("persistExtractionResult() error = %v", err)
	}

	updated, err := svc.GetPaper(paper.ID)
	if err != nil {
		t.Fatalf("GetPaper() error = %v", err)
	}
	if len(updated.Figures) != 1 {
		t.Fatalf("GetPaper() figures = %d, want 1", len(updated.Figures))
	}
	if updated.Figures[0].Source != figureSourceAuto {
		t.Fatalf("figure source = %q, want %q", updated.Figures[0].Source, figureSourceAuto)
	}
}

func TestUploadPaperRejectsDuplicatePDFAndReturnsExistingPaper(t *testing.T) {
	svc, repo, cfg := newTestService(t)

	content := []byte("%PDF-1.4 duplicate test")
	existing, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Existing",
		OriginalFilename: "existing.pdf",
		StoredPDFName:    "existing.pdf",
		FileSize:         int64(len(content)),
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.PapersDir(), existing.StoredPDFName), content, 0o644); err != nil {
		t.Fatalf("WriteFile(existing pdf) error = %v", err)
	}
	if err := svc.backfillPaperChecksums(); err != nil {
		t.Fatalf("backfillPaperChecksums() error = %v", err)
	}

	file := &testMultipartFile{Reader: bytes.NewReader(content)}
	header := &multipart.FileHeader{
		Filename: "duplicate.pdf",
		Size:     int64(len(content)),
		Header: textproto.MIMEHeader{
			"Content-Type": []string{"application/pdf"},
		},
	}

	_, err = svc.UploadPaper(file, header, UploadPaperParams{Title: "Duplicate"})
	var duplicateErr *DuplicatePaperError
	if !errors.As(err, &duplicateErr) {
		t.Fatalf("UploadPaper() error = %T %v, want DuplicatePaperError", err, err)
	}
	if duplicateErr.Paper == nil || duplicateErr.Paper.ID != existing.ID {
		t.Fatalf("DuplicatePaperError paper = %+v, want existing paper id %d", duplicateErr.Paper, existing.ID)
	}
	if !apperr.IsCode(err, apperr.CodeConflict) {
		t.Fatalf("UploadPaper() code = %q, want %q", apperr.CodeOf(err), apperr.CodeConflict)
	}
}

func TestDeleteFigureRemovesFileAndReturnsUpdatedPaper(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	paper := createTestPaper(t, repo)

	figurePath := filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename)
	if err := os.WriteFile(figurePath, []byte("img"), 0o644); err != nil {
		t.Fatalf("WriteFile(figure) error = %v", err)
	}

	updated, err := svc.DeleteFigure(paper.Figures[0].ID)
	if err != nil {
		t.Fatalf("DeleteFigure() error = %v", err)
	}
	if len(updated.Figures) != 0 {
		t.Fatalf("DeleteFigure() figures = %d, want 0", len(updated.Figures))
	}
	if _, err := os.Stat(figurePath); !os.IsNotExist(err) {
		t.Fatalf("figure file still exists, stat err = %v", err)
	}
}

func TestCreateSubfiguresAssignsLabelAndDecoratesParent(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	paper := createTestPaper(t, repo)

	parentPath := filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename)
	parentData, err := decodeBase64(testPNGDataURL(t, 80, 60))
	if err != nil {
		t.Fatalf("decodeBase64() error = %v", err)
	}
	if err := os.WriteFile(parentPath, parentData, 0o644); err != nil {
		t.Fatalf("WriteFile(parent figure) error = %v", err)
	}

	updated, addedCount, err := svc.CreateSubfigures(paper.Figures[0].ID, CreateSubfiguresParams{
		Regions: []model.SubfigureExtractionRegion{
			{
				X:         0.12,
				Y:         0.18,
				Width:     0.4,
				Height:    0.45,
				ImageData: testPNGDataURL(t, 20, 16),
				Caption:   "Panel A",
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateSubfigures() error = %v", err)
	}
	if addedCount != 1 {
		t.Fatalf("CreateSubfigures() addedCount = %d, want 1", addedCount)
	}
	if len(updated.Figures) != 2 {
		t.Fatalf("CreateSubfigures() figures = %d, want 2", len(updated.Figures))
	}

	var parentFigure *model.Figure
	var childFigure *model.Figure
	for i := range updated.Figures {
		figure := &updated.Figures[i]
		if figure.ID == paper.Figures[0].ID {
			parentFigure = figure
		}
		if figure.ParentFigureID != nil {
			childFigure = figure
		}
	}
	if parentFigure == nil || childFigure == nil {
		t.Fatalf("CreateSubfigures() figures = %+v, want parent and child", updated.Figures)
	}
	if childFigure.SubfigureLabel != "a" {
		t.Fatalf("CreateSubfigures() subfigure_label = %q, want %q", childFigure.SubfigureLabel, "a")
	}
	if childFigure.DisplayLabel != "Fig 1a" {
		t.Fatalf("CreateSubfigures() display_label = %q, want %q", childFigure.DisplayLabel, "Fig 1a")
	}
	if childFigure.ParentDisplayLabel != "Fig 1" {
		t.Fatalf("CreateSubfigures() parent_display_label = %q, want %q", childFigure.ParentDisplayLabel, "Fig 1")
	}
	if len(parentFigure.Subfigures) != 1 || parentFigure.Subfigures[0].ID != childFigure.ID {
		t.Fatalf("CreateSubfigures() parent subfigures = %+v, want child %d", parentFigure.Subfigures, childFigure.ID)
	}
	if !strings.HasPrefix(childFigure.Filename, virtualSubfigureFilenamePrefix) {
		t.Fatalf("CreateSubfigures() filename = %q, want virtual metadata filename", childFigure.Filename)
	}
	if childFigure.ImageURL != "/api/figures/"+strconv.FormatInt(childFigure.ID, 10)+"/image" {
		t.Fatalf("CreateSubfigures() image_url = %q, want dynamic image route", childFigure.ImageURL)
	}
}

func TestCreateSubfiguresUsesManualLabelAndAutoFallback(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	paper := createTestPaper(t, repo)

	parentPath := filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename)
	parentData, err := decodeBase64(testPNGDataURL(t, 80, 60))
	if err != nil {
		t.Fatalf("decodeBase64() error = %v", err)
	}
	if err := os.WriteFile(parentPath, parentData, 0o644); err != nil {
		t.Fatalf("WriteFile(parent figure) error = %v", err)
	}

	updated, addedCount, err := svc.CreateSubfigures(paper.Figures[0].ID, CreateSubfiguresParams{
		Regions: []model.SubfigureExtractionRegion{
			{
				X:      0.08,
				Y:      0.10,
				Width:  0.22,
				Height: 0.28,
				Label:  "B",
			},
			{
				X:      0.40,
				Y:      0.20,
				Width:  0.20,
				Height: 0.25,
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateSubfigures() error = %v", err)
	}
	if addedCount != 2 {
		t.Fatalf("CreateSubfigures() addedCount = %d, want 2", addedCount)
	}

	var labels []string
	var displayLabels []string
	for _, figure := range updated.Figures {
		if figure.ParentFigureID == nil {
			continue
		}
		labels = append(labels, figure.SubfigureLabel)
		displayLabels = append(displayLabels, figure.DisplayLabel)
	}
	if !containsString(labels, "b") {
		t.Fatalf("CreateSubfigures() labels = %+v, want normalized manual label b", labels)
	}
	if !containsString(labels, "a") {
		t.Fatalf("CreateSubfigures() labels = %+v, want auto fallback label a", labels)
	}
	if !containsString(displayLabels, "Fig 1b") {
		t.Fatalf("CreateSubfigures() displayLabels = %+v, want manual display label Fig 1b", displayLabels)
	}
	if !containsString(displayLabels, "Fig 1a") {
		t.Fatalf("CreateSubfigures() displayLabels = %+v, want auto display label", displayLabels)
	}
}

func TestCreateSubfiguresAllowsAddingAAfterExistingB(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	paper := createTestPaper(t, repo)

	parentPath := filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename)
	parentData, err := decodeBase64(testPNGDataURL(t, 80, 60))
	if err != nil {
		t.Fatalf("decodeBase64() error = %v", err)
	}
	if err := os.WriteFile(parentPath, parentData, 0o644); err != nil {
		t.Fatalf("WriteFile(parent figure) error = %v", err)
	}

	if _, addedCount, err := svc.CreateSubfigures(paper.Figures[0].ID, CreateSubfiguresParams{
		Regions: []model.SubfigureExtractionRegion{
			{
				X:      0.08,
				Y:      0.10,
				Width:  0.22,
				Height: 0.28,
				Label:  "b",
			},
		},
	}); err != nil {
		t.Fatalf("CreateSubfigures(first) error = %v", err)
	} else if addedCount != 1 {
		t.Fatalf("CreateSubfigures(first) addedCount = %d, want 1", addedCount)
	}

	updated, addedCount, err := svc.CreateSubfigures(paper.Figures[0].ID, CreateSubfiguresParams{
		Regions: []model.SubfigureExtractionRegion{
			{
				X:      0.40,
				Y:      0.20,
				Width:  0.20,
				Height: 0.25,
				Label:  "a",
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateSubfigures(second) error = %v", err)
	}
	if addedCount != 1 {
		t.Fatalf("CreateSubfigures(second) addedCount = %d, want 1", addedCount)
	}

	var labels []string
	for _, figure := range updated.Figures {
		if figure.ParentFigureID == nil {
			continue
		}
		labels = append(labels, figure.SubfigureLabel)
	}
	if !containsString(labels, "a") || !containsString(labels, "b") {
		t.Fatalf("CreateSubfigures(second) labels = %+v, want both a and b", labels)
	}
}

func TestGetFigureImageRendersSubfigureCrop(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	paper := createTestPaper(t, repo)

	parentPath := filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename)
	parentData, err := decodeBase64(testPNGDataURL(t, 100, 80))
	if err != nil {
		t.Fatalf("decodeBase64() error = %v", err)
	}
	if err := os.WriteFile(parentPath, parentData, 0o644); err != nil {
		t.Fatalf("WriteFile(parent figure) error = %v", err)
	}

	updated, _, err := svc.CreateSubfigures(paper.Figures[0].ID, CreateSubfiguresParams{
		Regions: []model.SubfigureExtractionRegion{
			{
				X:       0.1,
				Y:       0.2,
				Width:   0.3,
				Height:  0.25,
				Caption: "Panel A",
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateSubfigures() error = %v", err)
	}

	var childFigure *model.Figure
	for i := range updated.Figures {
		if updated.Figures[i].ParentFigureID != nil {
			childFigure = &updated.Figures[i]
			break
		}
	}
	if childFigure == nil {
		t.Fatalf("CreateSubfigures() figures = %+v, want child figure", updated.Figures)
	}

	data, contentType, filename, err := svc.GetFigureImage(childFigure.ID)
	if err != nil {
		t.Fatalf("GetFigureImage() error = %v", err)
	}
	if contentType != "image/png" {
		t.Fatalf("GetFigureImage() content_type = %q, want %q", contentType, "image/png")
	}
	if filename != "figure-original_a.png" {
		t.Fatalf("GetFigureImage() filename = %q, want %q", filename, "figure-original_a.png")
	}

	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("png.Decode() error = %v", err)
	}
	if got := img.Bounds().Dx(); got != 30 {
		t.Fatalf("GetFigureImage() width = %d, want 30", got)
	}
	if got := img.Bounds().Dy(); got != 20 {
		t.Fatalf("GetFigureImage() height = %d, want 20", got)
	}
}

func TestCreateSubfiguresRejectsNonAlphabeticManualLabel(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	paper := createTestPaper(t, repo)

	parentPath := filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename)
	parentData, err := decodeBase64(testPNGDataURL(t, 64, 48))
	if err != nil {
		t.Fatalf("decodeBase64() error = %v", err)
	}
	if err := os.WriteFile(parentPath, parentData, 0o644); err != nil {
		t.Fatalf("WriteFile(parent figure) error = %v", err)
	}

	_, _, err = svc.CreateSubfigures(paper.Figures[0].ID, CreateSubfiguresParams{
		Regions: []model.SubfigureExtractionRegion{
			{
				X:      0.12,
				Y:      0.15,
				Width:  0.25,
				Height: 0.25,
				Label:  "12345",
			},
		},
	})
	if !apperr.IsCode(err, apperr.CodeInvalidArgument) {
		t.Fatalf("CreateSubfigures() code = %q, want %q", apperr.CodeOf(err), apperr.CodeInvalidArgument)
	}
}

func TestDeleteFigureRemovesParentFileForSubfigureBranch(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	paper := createTestPaper(t, repo)

	parentPath := filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename)
	parentData, err := decodeBase64(testPNGDataURL(t, 64, 48))
	if err != nil {
		t.Fatalf("decodeBase64() error = %v", err)
	}
	if err := os.WriteFile(parentPath, parentData, 0o644); err != nil {
		t.Fatalf("WriteFile(parent figure) error = %v", err)
	}

	updated, _, err := svc.CreateSubfigures(paper.Figures[0].ID, CreateSubfiguresParams{
		Regions: []model.SubfigureExtractionRegion{
			{
				X:         0.1,
				Y:         0.1,
				Width:     0.35,
				Height:    0.35,
				ImageData: testPNGDataURL(t, 18, 12),
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateSubfigures() error = %v", err)
	}

	var childFigure *model.Figure
	for i := range updated.Figures {
		if updated.Figures[i].ParentFigureID != nil {
			childFigure = &updated.Figures[i]
			break
		}
	}
	if childFigure == nil {
		t.Fatalf("CreateSubfigures() missing child figure: %+v", updated.Figures)
	}
	if !strings.HasPrefix(childFigure.Filename, virtualSubfigureFilenamePrefix) {
		t.Fatalf("CreateSubfigures() filename = %q, want virtual metadata filename", childFigure.Filename)
	}

	result, err := svc.DeleteFigure(paper.Figures[0].ID)
	if err != nil {
		t.Fatalf("DeleteFigure() error = %v", err)
	}
	if len(result.Figures) != 0 {
		t.Fatalf("DeleteFigure() figures = %d, want 0", len(result.Figures))
	}
	if _, err := os.Stat(parentPath); !os.IsNotExist(err) {
		t.Fatalf("parent figure file still exists, stat err = %v", err)
	}
}

func TestCreateOrUpdateFigurePaletteBindsPaletteToSubfigure(t *testing.T) {
	svc, repo, _ := newTestService(t)
	paper := createTestPaper(t, repo)

	updated, _, err := svc.CreateSubfigures(paper.Figures[0].ID, CreateSubfiguresParams{
		Regions: []model.SubfigureExtractionRegion{
			{
				X:         0.15,
				Y:         0.2,
				Width:     0.3,
				Height:    0.35,
				ImageData: testPNGDataURL(t, 24, 18),
				Caption:   "Panel A",
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateSubfigures() error = %v", err)
	}

	var childFigure *model.Figure
	for i := range updated.Figures {
		if updated.Figures[i].ParentFigureID != nil {
			childFigure = &updated.Figures[i]
			break
		}
	}
	if childFigure == nil {
		t.Fatalf("CreateSubfigures() figures = %+v, want child figure", updated.Figures)
	}

	palette, refreshedPaper, err := svc.CreateOrUpdateFigurePalette(childFigure.ID, CreatePaletteParams{
		Colors: []string{"#aabbcc", "#DDEEFF", "#ddeeff"},
	})
	if err != nil {
		t.Fatalf("CreateOrUpdateFigurePalette() error = %v", err)
	}
	if palette.Name != "Fig 1a 配色" {
		t.Fatalf("CreateOrUpdateFigurePalette() name = %q, want %q", palette.Name, "Fig 1a 配色")
	}
	if len(palette.Colors) != 2 || palette.Colors[0] != "#AABBCC" || palette.Colors[1] != "#DDEEFF" {
		t.Fatalf("CreateOrUpdateFigurePalette() colors = %+v, want normalized unique colors", palette.Colors)
	}

	var refreshedChild *model.Figure
	for i := range refreshedPaper.Figures {
		if refreshedPaper.Figures[i].ID == childFigure.ID {
			refreshedChild = &refreshedPaper.Figures[i]
			break
		}
	}
	if refreshedChild == nil {
		t.Fatalf("CreateOrUpdateFigurePalette() paper figures = %+v, want refreshed child", refreshedPaper.Figures)
	}
	if refreshedChild.PaletteCount != 1 {
		t.Fatalf("CreateOrUpdateFigurePalette() palette_count = %d, want 1", refreshedChild.PaletteCount)
	}
	if refreshedChild.PaletteName != "Fig 1a 配色" {
		t.Fatalf("CreateOrUpdateFigurePalette() palette_name = %q, want %q", refreshedChild.PaletteName, "Fig 1a 配色")
	}
	if len(refreshedChild.PaletteColors) != 2 || refreshedChild.PaletteColors[0] != "#AABBCC" || refreshedChild.PaletteColors[1] != "#DDEEFF" {
		t.Fatalf("CreateOrUpdateFigurePalette() palette_colors = %+v, want persisted colors", refreshedChild.PaletteColors)
	}
}

func TestCreateOrUpdateFigurePaletteRejectsParentFigure(t *testing.T) {
	svc, repo, _ := newTestService(t)
	paper := createTestPaper(t, repo)

	_, _, err := svc.CreateOrUpdateFigurePalette(paper.Figures[0].ID, CreatePaletteParams{
		Colors: []string{"#112233"},
	})
	if !apperr.IsCode(err, apperr.CodeFailedPrecondition) {
		t.Fatalf("CreateOrUpdateFigurePalette() code = %q, want %q", apperr.CodeOf(err), apperr.CodeFailedPrecondition)
	}
}

func TestListFiguresExcludesSubfigures(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	paper := createTestPaper(t, repo)

	parentPath := filepath.Join(cfg.FiguresDir(), paper.Figures[0].Filename)
	parentData, err := decodeBase64(testPNGDataURL(t, 60, 40))
	if err != nil {
		t.Fatalf("decodeBase64() error = %v", err)
	}
	if err := os.WriteFile(parentPath, parentData, 0o644); err != nil {
		t.Fatalf("WriteFile(parent figure) error = %v", err)
	}

	if _, _, err := svc.CreateSubfigures(paper.Figures[0].ID, CreateSubfiguresParams{
		Regions: []model.SubfigureExtractionRegion{
			{
				X:      0.15,
				Y:      0.2,
				Width:  0.3,
				Height: 0.35,
			},
		},
	}); err != nil {
		t.Fatalf("CreateSubfigures() error = %v", err)
	}

	result, err := svc.ListFigures(model.FigureFilter{})
	if err != nil {
		t.Fatalf("ListFigures() error = %v", err)
	}
	if result.Total != 1 || len(result.Figures) != 1 {
		t.Fatalf("ListFigures() total=%d len=%d, want 1/1", result.Total, len(result.Figures))
	}
	if result.Figures[0].ParentFigureID != nil {
		t.Fatalf("ListFigures() returned subfigure: %+v", result.Figures[0])
	}
}

func TestNormalizeManualRegionRejectsMissingImageData(t *testing.T) {
	if _, err := normalizeManualRegion(model.ManualExtractionRegion{
		PageNumber: 1,
		X:          0.1,
		Y:          0.1,
		Width:      0.4,
		Height:     0.4,
	}); !apperr.IsCode(err, apperr.CodeInvalidArgument) {
		t.Fatalf("normalizeManualRegion() code = %q, want %q", apperr.CodeOf(err), apperr.CodeInvalidArgument)
	}
}

func TestNormalizeManualRegionMapsLLMSourceToAuto(t *testing.T) {
	region, err := normalizeManualRegion(model.ManualExtractionRegion{
		PageNumber: 1,
		X:          0.1,
		Y:          0.1,
		Width:      0.4,
		Height:     0.4,
		Source:     manualFigureSourceLLM,
		ImageData:  testPNGDataURL(t, 12, 10),
	})
	if err != nil {
		t.Fatalf("normalizeManualRegion() error = %v", err)
	}
	if region.Source != figureSourceAuto {
		t.Fatalf("normalizeManualRegion() source = %q, want %q", region.Source, figureSourceAuto)
	}
}

func TestManualExtractFiguresStoresClientRenderedImage(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	paper := createTestPaper(t, repo)

	if err := os.WriteFile(filepath.Join(cfg.PapersDir(), paper.StoredPDFName), []byte("%PDF-1.4 test"), 0o644); err != nil {
		t.Fatalf("WriteFile(pdf) error = %v", err)
	}

	updated, addedCount, err := svc.ManualExtractFigures(paper.ID, ManualExtractParams{
		Regions: []model.ManualExtractionRegion{
			{
				PageNumber: 1,
				X:          0.1,
				Y:          0.2,
				Width:      0.3,
				Height:     0.4,
				ImageData:  testPNGDataURL(t, 24, 18),
				Caption:    "Manual figure",
			},
		},
	})
	if err != nil {
		t.Fatalf("ManualExtractFigures() error = %v", err)
	}

	if addedCount != 1 {
		t.Fatalf("ManualExtractFigures() addedCount = %d, want 1", addedCount)
	}
	if len(updated.Figures) != 2 {
		t.Fatalf("ManualExtractFigures() figures = %d, want 2", len(updated.Figures))
	}

	var manualFigure *model.Figure
	for i := range updated.Figures {
		if updated.Figures[i].Source == "manual" {
			manualFigure = &updated.Figures[i]
			break
		}
	}
	if manualFigure == nil {
		t.Fatalf("ManualExtractFigures() missing manual figure: %+v", updated.Figures)
	}
	if manualFigure.Caption != "Manual figure" {
		t.Fatalf("ManualExtractFigures() caption = %q, want %q", manualFigure.Caption, "Manual figure")
	}
	if !strings.HasSuffix(manualFigure.Filename, ".png") {
		t.Fatalf("ManualExtractFigures() filename = %q, want .png suffix", manualFigure.Filename)
	}
	if _, err := os.Stat(filepath.Join(cfg.FiguresDir(), manualFigure.Filename)); err != nil {
		t.Fatalf("stored manual figure missing, stat err = %v", err)
	}
}

func TestMigrateLegacyManualPendingPapersMarksCompleted(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{
		StorageDir:              filepath.Join(root, "storage"),
		DatabasePath:            filepath.Join(root, "library.db"),
		MaxUploadSize:           10 << 20,
		AdminUsername:           "citebox",
		AdminPassword:           "citebox123",
		ExtractorTimeoutSeconds: 1,
		ExtractorPollInterval:   1,
		ExtractorFileField:      "file",
	}

	repo, err := repository.NewLibraryRepository(cfg.DatabasePath)
	if err != nil {
		t.Fatalf("NewLibraryRepository() error = %v", err)
	}
	t.Cleanup(func() {
		_ = repo.Close()
	})

	if _, err := repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "Legacy Manual Workflow",
		OriginalFilename: "legacy-manual-workflow.pdf",
		StoredPDFName:    "legacy_manual_workflow.pdf",
		FileSize:         256,
		ContentType:      "application/pdf",
		ExtractionStatus: manualPendingStatus,
		ExtractorMessage: "未配置自动解析服务，请直接进入人工处理",
	}); err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := NewLibraryService(repo, cfg, WithLogger(logger), WithoutBackgroundJobs())
	if err != nil {
		t.Fatalf("NewLibraryService() error = %v", err)
	}

	result, err := svc.ListPapers(model.PaperFilter{Status: "completed"})
	if err != nil {
		t.Fatalf("ListPapers() error = %v", err)
	}
	if result.Total != 1 || len(result.Papers) != 1 {
		t.Fatalf("ListPapers() total=%d len=%d, want 1/1", result.Total, len(result.Papers))
	}
	if !strings.Contains(result.Papers[0].ExtractorMessage, "文献已入库") {
		t.Fatalf("ListPapers() extractor_message = %q, want migrated library-ready hint", result.Papers[0].ExtractorMessage)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
