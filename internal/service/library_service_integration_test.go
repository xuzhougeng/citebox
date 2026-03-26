package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	wolaiapi "github.com/xuzhougeng/citebox/internal/wolai"
)

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
	if settings.DailyRecommendation.Enabled {
		t.Fatalf("GetWeixinBridgeSettings() daily_recommendation.enabled = %v, want false by default", settings.DailyRecommendation.Enabled)
	}
	if settings.DailyRecommendation.SendTime != model.DefaultWeixinDailyRecommendationSendTime {
		t.Fatalf("GetWeixinBridgeSettings() daily_recommendation.send_time = %q, want %q", settings.DailyRecommendation.SendTime, model.DefaultWeixinDailyRecommendationSendTime)
	}
}

func TestUpdateWeixinBridgeSettingsPersistsAndAppearsInAuthSettings(t *testing.T) {
	svc, repo, _ := newTestService(t)

	settings, err := svc.UpdateWeixinBridgeSettings(model.WeixinBridgeSettings{
		Enabled: true,
		DailyRecommendation: model.WeixinDailyRecommendationSettings{
			Enabled:  true,
			SendTime: "08:30",
		},
	})
	if err != nil {
		t.Fatalf("UpdateWeixinBridgeSettings() error = %v", err)
	}
	if !settings.Enabled {
		t.Fatalf("UpdateWeixinBridgeSettings() enabled = %v, want true", settings.Enabled)
	}
	if !settings.DailyRecommendation.Enabled || settings.DailyRecommendation.SendTime != "08:30" {
		t.Fatalf("UpdateWeixinBridgeSettings() daily_recommendation = %+v, want enabled at 08:30", settings.DailyRecommendation)
	}

	reloaded, err := svc.GetWeixinBridgeSettings()
	if err != nil {
		t.Fatalf("GetWeixinBridgeSettings() reload error = %v", err)
	}
	if !reloaded.Enabled {
		t.Fatalf("GetWeixinBridgeSettings() reload enabled = %v, want true", reloaded.Enabled)
	}
	if !reloaded.DailyRecommendation.Enabled || reloaded.DailyRecommendation.SendTime != "08:30" {
		t.Fatalf("GetWeixinBridgeSettings() reload daily_recommendation = %+v, want enabled at 08:30", reloaded.DailyRecommendation)
	}

	authSettings := svc.GetAuthSettings()
	if !authSettings.WeixinBridge.Enabled {
		t.Fatalf("GetAuthSettings() weixin_bridge = %+v, want enabled", authSettings.WeixinBridge)
	}
	if !authSettings.WeixinBridge.DailyRecommendation.Enabled || authSettings.WeixinBridge.DailyRecommendation.SendTime != "08:30" {
		t.Fatalf("GetAuthSettings() weixin_bridge.daily_recommendation = %+v, want enabled at 08:30", authSettings.WeixinBridge.DailyRecommendation)
	}

	raw, err := repo.GetAppSetting(weixinBridgeSettingsKey)
	if err != nil {
		t.Fatalf("GetAppSetting(%q) error = %v", weixinBridgeSettingsKey, err)
	}
	if !strings.Contains(raw, `"enabled":true`) || !strings.Contains(raw, `"daily_recommendation":{"enabled":true,"send_time":"08:30"}`) {
		t.Fatalf("saved weixin bridge settings = %q, want bridge and daily recommendation persisted", raw)
	}
}

func TestUpdateWeixinBridgeSettingsDefaultsDailyRecommendationTimeWhenBlank(t *testing.T) {
	svc, _, _ := newTestService(t)

	settings, err := svc.UpdateWeixinBridgeSettings(model.WeixinBridgeSettings{
		DailyRecommendation: model.WeixinDailyRecommendationSettings{
			Enabled: true,
		},
	})
	if err != nil {
		t.Fatalf("UpdateWeixinBridgeSettings() error = %v", err)
	}
	if settings.DailyRecommendation.SendTime != model.DefaultWeixinDailyRecommendationSendTime {
		t.Fatalf("UpdateWeixinBridgeSettings() daily_recommendation.send_time = %q, want %q", settings.DailyRecommendation.SendTime, model.DefaultWeixinDailyRecommendationSendTime)
	}
}

func TestGetTTSSettingsDefaultsResourceID(t *testing.T) {
	svc, _, _ := newTestService(t)

	settings, err := svc.GetTTSSettings()
	if err != nil {
		t.Fatalf("GetTTSSettings() error = %v", err)
	}
	if settings.ResourceID != doubaoTTSDefaultResourceID {
		t.Fatalf("GetTTSSettings() resource_id = %q, want %q", settings.ResourceID, doubaoTTSDefaultResourceID)
	}
	if !settings.WeixinVoiceOutputEnabled {
		t.Fatalf("GetTTSSettings() weixin_voice_output_enabled = %v, want true", settings.WeixinVoiceOutputEnabled)
	}
}

func TestUpdateTTSSettingsPersistsSeparately(t *testing.T) {
	svc, repo, _ := newTestService(t)

	settings, err := svc.UpdateTTSSettings(model.TTSSettings{
		AppID:      " app-id ",
		AccessKey:  " access-key ",
		ResourceID: " ",
		Speaker:    " speaker-id ",
	})
	if err != nil {
		t.Fatalf("UpdateTTSSettings() error = %v", err)
	}
	if settings.AppID != "app-id" || settings.AccessKey != "access-key" || settings.Speaker != "speaker-id" {
		t.Fatalf("UpdateTTSSettings() settings = %+v, want trimmed values", settings)
	}
	if settings.ResourceID != doubaoTTSDefaultResourceID {
		t.Fatalf("UpdateTTSSettings() resource_id = %q, want %q", settings.ResourceID, doubaoTTSDefaultResourceID)
	}
	if !settings.WeixinVoiceOutputEnabled {
		t.Fatalf("UpdateTTSSettings() weixin_voice_output_enabled = %v, want true", settings.WeixinVoiceOutputEnabled)
	}

	reloaded, err := svc.GetTTSSettings()
	if err != nil {
		t.Fatalf("GetTTSSettings() reload error = %v", err)
	}
	if reloaded.AppID != "app-id" || reloaded.AccessKey != "access-key" || reloaded.Speaker != "speaker-id" {
		t.Fatalf("GetTTSSettings() reload = %+v, want persisted values", reloaded)
	}
	if !reloaded.WeixinVoiceOutputEnabled {
		t.Fatalf("GetTTSSettings() reload weixin_voice_output_enabled = %v, want true", reloaded.WeixinVoiceOutputEnabled)
	}

	raw, err := repo.GetAppSetting(ttsSettingsKey)
	if err != nil {
		t.Fatalf("GetAppSetting(%q) error = %v", ttsSettingsKey, err)
	}
	if !strings.Contains(raw, `"app_id":"app-id"`) || !strings.Contains(raw, `"speaker":"speaker-id"`) || !strings.Contains(raw, `"weixin_voice_output_enabled":true`) {
		t.Fatalf("saved tts settings = %q, want TTS fields persisted", raw)
	}
}

func TestSetWeixinVoiceOutputEnabledPersistsSeparately(t *testing.T) {
	svc, repo, _ := newTestService(t)

	settings, err := svc.SetWeixinVoiceOutputEnabled(false)
	if err != nil {
		t.Fatalf("SetWeixinVoiceOutputEnabled(false) error = %v", err)
	}
	if settings.WeixinVoiceOutputEnabled {
		t.Fatalf("SetWeixinVoiceOutputEnabled(false) = %+v, want weixin voice output disabled", settings)
	}

	reloaded, err := svc.GetTTSSettings()
	if err != nil {
		t.Fatalf("GetTTSSettings() reload error = %v", err)
	}
	if reloaded.WeixinVoiceOutputEnabled {
		t.Fatalf("GetTTSSettings() reload = %+v, want weixin voice output disabled", reloaded)
	}

	raw, err := repo.GetAppSetting(ttsSettingsKey)
	if err != nil {
		t.Fatalf("GetAppSetting(%q) error = %v", ttsSettingsKey, err)
	}
	if !strings.Contains(raw, `"weixin_voice_output_enabled":false`) {
		t.Fatalf("saved tts settings = %q, want weixin voice output flag persisted", raw)
	}
}

func TestGetTTSSettingsFallsBackToLegacyWeixinBridgeFields(t *testing.T) {
	svc, repo, _ := newTestService(t)

	if err := repo.UpsertAppSetting(weixinBridgeSettingsKey, `{"enabled":true,"tts_app_id":"legacy-app","tts_access_key":"legacy-key","tts_resource_id":"legacy-resource","tts_speaker":"legacy-speaker"}`); err != nil {
		t.Fatalf("UpsertAppSetting() error = %v", err)
	}

	settings, err := svc.GetTTSSettings()
	if err != nil {
		t.Fatalf("GetTTSSettings() error = %v", err)
	}
	if settings.AppID != "legacy-app" || settings.AccessKey != "legacy-key" || settings.ResourceID != "legacy-resource" || settings.Speaker != "legacy-speaker" {
		t.Fatalf("GetTTSSettings() = %+v, want legacy values", settings)
	}
	if !settings.WeixinVoiceOutputEnabled {
		t.Fatalf("GetTTSSettings() legacy weixin_voice_output_enabled = %v, want true", settings.WeixinVoiceOutputEnabled)
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
