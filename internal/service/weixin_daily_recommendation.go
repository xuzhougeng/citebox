package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/weixin"
)

const (
	weixinDailyRecommendationStateKey   = "weixin_daily_recommendation_state"
	weixinDailyRecommendationDateLayout = "2006-01-02"
	weixinDailyRecommendationCandidates = 24
)

type weixinDailyRecommendationState struct {
	LastSentDate string `json:"last_sent_date"`
	LastSentAt   string `json:"last_sent_at,omitempty"`
	LastFigureID int64  `json:"last_figure_id,omitempty"`
}

type weixinDailyRecommendationSendResult struct {
	FigureID     int64
	PaperTitle   string
	DisplayLabel string
	SentAt       time.Time
	Test         bool
}

func (s *LibraryService) TestWeixinDailyRecommendation(ctx context.Context, input model.WeixinDailyRecommendationSettings) (string, error) {
	settings, err := validateWeixinDailyRecommendationSettings(input)
	if err != nil {
		return "", err
	}

	result, err := s.sendWeixinDailyRecommendation(ctx, settings, weixinDailyRecommendationSendOptions{
		Now:              time.Now(),
		IsTest:           true,
		PersistSendState: false,
		RequireBridgeOn:  false,
	})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("测试图片已发送到微信：%s", result.summaryLabel()), nil
}

func (s *LibraryService) MaybeSendWeixinDailyRecommendation(ctx context.Context, now time.Time) (*weixinDailyRecommendationSendResult, error) {
	settings, err := s.GetWeixinBridgeSettings()
	if err != nil {
		return nil, err
	}
	if settings == nil || !settings.Enabled || !settings.DailyRecommendation.Enabled {
		return nil, nil
	}

	normalizedNow := now.In(time.Local)
	state, err := s.loadWeixinDailyRecommendationState()
	if err != nil {
		return nil, err
	}
	if !shouldSendWeixinDailyRecommendation(normalizedNow, settings.DailyRecommendation.SendTime, state.LastSentDate) {
		return nil, nil
	}

	return s.sendWeixinDailyRecommendation(ctx, settings.DailyRecommendation, weixinDailyRecommendationSendOptions{
		Now:              normalizedNow,
		IsTest:           false,
		PersistSendState: true,
		RequireBridgeOn:  true,
	})
}

type weixinDailyRecommendationSendOptions struct {
	Now              time.Time
	IsTest           bool
	PersistSendState bool
	RequireBridgeOn  bool
}

func (s *LibraryService) sendWeixinDailyRecommendation(
	ctx context.Context,
	settings model.WeixinDailyRecommendationSettings,
	options weixinDailyRecommendationSendOptions,
) (*weixinDailyRecommendationSendResult, error) {
	if options.Now.IsZero() {
		options.Now = time.Now()
	}

	s.weixinRecommendMu.Lock()
	defer s.weixinRecommendMu.Unlock()

	if options.RequireBridgeOn {
		bridgeSettings, err := s.GetWeixinBridgeSettings()
		if err != nil {
			return nil, err
		}
		if bridgeSettings == nil || !bridgeSettings.Enabled {
			return nil, nil
		}
	}

	binding, err := s.loadWeixinBinding()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(binding.Token) == "" || strings.TrimSpace(binding.UserID) == "" {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "请先完成微信绑定")
	}

	figure, imagePath, cleanup, err := s.pickWeixinDailyRecommendationFigure(ctx)
	if err != nil {
		return nil, err
	}
	if cleanup == nil {
		cleanup = func() {}
	}
	defer cleanup()

	client := weixin.NewClient(binding.BaseURL, binding.Token, nil)
	message := formatWeixinDailyRecommendationMessage(figure, options.IsTest)
	if err := sendWeixinTextReply(ctx, client, binding.UserID, message, ""); err != nil {
		return nil, apperr.New(apperr.CodeUnavailable, fmt.Sprintf("发送今日推荐说明失败：%v", err))
	}
	if err := client.SendImageFile(ctx, binding.UserID, imagePath, ""); err != nil {
		return nil, apperr.New(apperr.CodeUnavailable, fmt.Sprintf("发送今日推荐图片失败：%v", err))
	}

	result := &weixinDailyRecommendationSendResult{
		FigureID:     figure.ID,
		PaperTitle:   strings.TrimSpace(figure.PaperTitle),
		DisplayLabel: formatFigureDisplayLabel(figure.FigureIndex, figure.SubfigureLabel),
		SentAt:       options.Now.In(time.Local),
		Test:         options.IsTest,
	}

	if options.PersistSendState {
		state := weixinDailyRecommendationState{
			LastSentDate: result.SentAt.Format(weixinDailyRecommendationDateLayout),
			LastSentAt:   result.SentAt.UTC().Format(time.RFC3339),
			LastFigureID: result.FigureID,
		}
		if err := s.saveWeixinDailyRecommendationState(state); err != nil {
			return nil, err
		}
	}

	return result, nil
}

func (s *LibraryService) pickWeixinDailyRecommendationFigure(ctx context.Context) (model.FigureListItem, string, func(), error) {
	candidateIDs, err := s.repo.ListRandomFigureIDs(weixinDailyRecommendationCandidates)
	if err != nil {
		return model.FigureListItem{}, "", nil, err
	}
	if len(candidateIDs) == 0 {
		return model.FigureListItem{}, "", nil, apperr.New(apperr.CodeFailedPrecondition, "当前还没有可发送的图片")
	}

	var lastErr error
	for _, figureID := range candidateIDs {
		if err := ctx.Err(); err != nil {
			return model.FigureListItem{}, "", nil, err
		}

		figure, err := s.repo.GetFigure(figureID)
		if err != nil {
			lastErr = err
			continue
		}
		if figure == nil {
			continue
		}

		data, _, filename, err := s.GetFigureImage(figureID)
		if err != nil {
			lastErr = err
			continue
		}

		path, cleanup, err := s.writeWeixinDailyRecommendationTempImage(data, filename)
		if err != nil {
			lastErr = err
			continue
		}

		return *figure, path, cleanup, nil
	}

	if lastErr != nil {
		return model.FigureListItem{}, "", nil, apperr.Wrap(apperr.CodeUnavailable, "可发送的图片文件当前不可用", lastErr)
	}
	return model.FigureListItem{}, "", nil, apperr.New(apperr.CodeFailedPrecondition, "当前还没有可发送的图片")
}

func (s *LibraryService) writeWeixinDailyRecommendationTempImage(data []byte, filename string) (string, func(), error) {
	stateDir := filepath.Join(s.config.StorageDir, weixinBridgeStateDirName)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return "", nil, apperr.Wrap(apperr.CodeInternal, "创建微信桥接缓存目录失败", err)
	}

	ext := filepath.Ext(strings.TrimSpace(filename))
	if ext == "" {
		ext = ".png"
	}

	file, err := os.CreateTemp(stateDir, "daily-recommendation-*"+ext)
	if err != nil {
		return "", nil, apperr.Wrap(apperr.CodeInternal, "创建今日推荐临时图片失败", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return "", nil, apperr.Wrap(apperr.CodeInternal, "写入今日推荐临时图片失败", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(file.Name())
		return "", nil, apperr.Wrap(apperr.CodeInternal, "保存今日推荐临时图片失败", err)
	}

	cleanup := func() {
		_ = os.Remove(file.Name())
	}
	return file.Name(), cleanup, nil
}

func formatWeixinDailyRecommendationMessage(figure model.FigureListItem, isTest bool) string {
	title := "今日推荐"
	if isTest {
		title = "今日推荐测试"
	}

	lines := []string{title}
	if paperTitle := strings.TrimSpace(figure.PaperTitle); paperTitle != "" {
		lines = append(lines, "文献："+paperTitle)
	}
	if label := formatFigureDisplayLabel(figure.FigureIndex, figure.SubfigureLabel); label != "" {
		lines = append(lines, "图片："+label)
	}
	if caption := clipRunes(strings.TrimSpace(figure.Caption), 160); caption != "" {
		lines = append(lines, "说明："+caption)
	}
	return strings.Join(lines, "\n")
}

func validateWeixinDailyRecommendationSettings(input model.WeixinDailyRecommendationSettings) (model.WeixinDailyRecommendationSettings, error) {
	sendTime, err := normalizeWeixinDailyRecommendationSendTime(input.SendTime)
	if err != nil {
		return model.WeixinDailyRecommendationSettings{}, err
	}
	return model.WeixinDailyRecommendationSettings{
		Enabled:  input.Enabled,
		SendTime: sendTime,
	}, nil
}

func shouldSendWeixinDailyRecommendation(now time.Time, sendTime, lastSentDate string) bool {
	current := now.In(time.Local)
	if strings.TrimSpace(lastSentDate) == current.Format(weixinDailyRecommendationDateLayout) {
		return false
	}

	triggerTime, err := time.Parse("15:04", strings.TrimSpace(sendTime))
	if err != nil {
		triggerTime, _ = time.Parse("15:04", model.DefaultWeixinDailyRecommendationSendTime)
	}

	currentMinutes := current.Hour()*60 + current.Minute()
	triggerMinutes := triggerTime.Hour()*60 + triggerTime.Minute()
	return currentMinutes >= triggerMinutes
}

func (s *LibraryService) loadWeixinDailyRecommendationState() (weixinDailyRecommendationState, error) {
	raw, err := s.repo.GetAppSetting(weixinDailyRecommendationStateKey)
	if err != nil {
		return weixinDailyRecommendationState{}, apperr.Wrap(apperr.CodeInternal, "读取今日推荐状态失败", err)
	}
	if strings.TrimSpace(raw) == "" {
		return weixinDailyRecommendationState{}, nil
	}

	var state weixinDailyRecommendationState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return weixinDailyRecommendationState{}, apperr.Wrap(apperr.CodeInternal, "解析今日推荐状态失败", err)
	}
	return state, nil
}

func (s *LibraryService) saveWeixinDailyRecommendationState(state weixinDailyRecommendationState) error {
	payload, err := json.Marshal(state)
	if err != nil {
		return apperr.Wrap(apperr.CodeInternal, "序列化今日推荐状态失败", err)
	}
	if err := s.repo.UpsertAppSetting(weixinDailyRecommendationStateKey, string(payload)); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "保存今日推荐状态失败", err)
	}
	return nil
}

func (r weixinDailyRecommendationSendResult) summaryLabel() string {
	label := strings.TrimSpace(r.DisplayLabel)
	if label != "" && strings.TrimSpace(r.PaperTitle) != "" {
		return fmt.Sprintf("%s · %s", r.PaperTitle, label)
	}
	if strings.TrimSpace(r.PaperTitle) != "" {
		return r.PaperTitle
	}
	if label != "" {
		return label
	}
	return fmt.Sprintf("图片 #%d", r.FigureID)
}
