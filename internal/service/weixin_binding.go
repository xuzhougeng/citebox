package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/weixin"
	qr "rsc.io/qr"
)

const weixinBindingKey = "weixin_binding"

type weixinBindingClient interface {
	BaseURL() string
	GetQRCode(ctx context.Context) (*weixin.QRCodeResponse, error)
	GetQRCodeStatus(ctx context.Context, qrcode string) (*weixin.QRCodeStatusResponse, error)
}

type weixinBindingRecord struct {
	Token     string `json:"token"`
	BaseURL   string `json:"base_url"`
	UserID    string `json:"user_id"`
	AccountID string `json:"account_id"`
	BoundAt   string `json:"bound_at"`
}

func defaultWeixinBindingClientFactory(token string) weixinBindingClient {
	return weixin.NewClient("", token, nil)
}

func (s *LibraryService) StartWeixinBinding(ctx context.Context) (model.WeixinBindingStartResponse, error) {
	client := s.newWeixinBindingClient("")
	qrCode, err := client.GetQRCode(ctx)
	if err != nil {
		return model.WeixinBindingStartResponse{}, apperr.Wrap(apperr.CodeUnavailable, "获取微信绑定二维码失败", err)
	}
	if qrCode.Ret != 0 {
		return model.WeixinBindingStartResponse{}, apperr.New(
			apperr.CodeUnavailable,
			firstNonEmpty(strings.TrimSpace(qrCode.Message), "微信二维码服务暂不可用"),
		)
	}

	qrSession := strings.TrimSpace(qrCode.QRCode)
	qrContent := strings.TrimSpace(qrCode.QRCodeImgContent)
	if qrSession == "" || qrContent == "" {
		return model.WeixinBindingStartResponse{}, apperr.New(apperr.CodeUnavailable, "微信二维码响应不完整，请稍后重试")
	}

	qrDataURL, err := renderWeixinQRCodeDataURL(qrContent)
	if err != nil {
		return model.WeixinBindingStartResponse{}, apperr.Wrap(apperr.CodeInternal, "生成微信二维码图片失败", err)
	}

	return model.WeixinBindingStartResponse{
		QRCode:        qrSession,
		QRCodeContent: qrContent,
		QRCodeDataURL: qrDataURL,
		Status:        "wait",
		Message:       "请使用微信扫码完成绑定",
	}, nil
}

func (s *LibraryService) GetWeixinBindingStatus(ctx context.Context, qrcode string) (model.WeixinBindingStatusResponse, error) {
	qrcode = strings.TrimSpace(qrcode)
	if qrcode == "" {
		return model.WeixinBindingStatusResponse{}, apperr.New(apperr.CodeInvalidArgument, "缺少二维码会话")
	}

	client := s.newWeixinBindingClient("")
	statusResp, err := client.GetQRCodeStatus(ctx, qrcode)
	if err != nil {
		return model.WeixinBindingStatusResponse{}, apperr.Wrap(apperr.CodeUnavailable, "查询微信绑定状态失败", err)
	}
	if statusResp.Ret != 0 && strings.TrimSpace(statusResp.Status) == "" {
		return model.WeixinBindingStatusResponse{}, apperr.New(
			apperr.CodeUnavailable,
			firstNonEmpty(strings.TrimSpace(statusResp.Message), "微信绑定状态查询失败"),
		)
	}

	status := normalizeWeixinBindingStatus(statusResp.Status)
	result := model.WeixinBindingStatusResponse{
		Status:  status,
		Message: weixinBindingStatusMessage(status, statusResp.Message),
		Binding: s.getWeixinBindingSummary(),
	}

	if status != "confirmed" {
		return result, nil
	}

	token := strings.TrimSpace(statusResp.BotToken)
	if token == "" {
		return model.WeixinBindingStatusResponse{}, apperr.New(apperr.CodeUnavailable, "微信绑定成功，但未返回有效凭证")
	}

	record := weixinBindingRecord{
		Token:     token,
		BaseURL:   firstNonEmpty(strings.TrimSpace(statusResp.BaseURL), client.BaseURL()),
		UserID:    strings.TrimSpace(statusResp.ILinkUserID),
		AccountID: strings.TrimSpace(statusResp.ILinkBotID),
		BoundAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if err := s.saveWeixinBinding(record); err != nil {
		return model.WeixinBindingStatusResponse{}, err
	}

	result.Binding = record.summary()
	result.Message = "微信绑定成功"
	return result, nil
}

func (s *LibraryService) getWeixinBindingSummary() model.WeixinBindingSummary {
	record, err := s.loadWeixinBinding()
	if err != nil {
		s.logger.Warn("load weixin binding failed", "error", err)
		return model.WeixinBindingSummary{}
	}
	return record.summary()
}

func (s *LibraryService) loadWeixinBinding() (weixinBindingRecord, error) {
	raw, err := s.repo.GetAppSetting(weixinBindingKey)
	if err != nil {
		return weixinBindingRecord{}, apperr.Wrap(apperr.CodeInternal, "读取微信绑定信息失败", err)
	}
	if strings.TrimSpace(raw) == "" {
		return weixinBindingRecord{}, nil
	}

	var record weixinBindingRecord
	if err := json.Unmarshal([]byte(raw), &record); err != nil {
		return weixinBindingRecord{}, apperr.Wrap(apperr.CodeInternal, "解析微信绑定信息失败", err)
	}
	return record, nil
}

func (s *LibraryService) saveWeixinBinding(record weixinBindingRecord) error {
	payload, err := json.Marshal(record)
	if err != nil {
		return apperr.Wrap(apperr.CodeInternal, "序列化微信绑定信息失败", err)
	}
	if err := s.repo.UpsertAppSetting(weixinBindingKey, string(payload)); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "保存微信绑定信息失败", err)
	}
	return nil
}

func (s *LibraryService) newWeixinBindingClient(token string) weixinBindingClient {
	if s.weixinClientFactory != nil {
		return s.weixinClientFactory(token)
	}
	return defaultWeixinBindingClientFactory(token)
}

func (r weixinBindingRecord) summary() model.WeixinBindingSummary {
	if strings.TrimSpace(r.Token) == "" && strings.TrimSpace(r.AccountID) == "" && strings.TrimSpace(r.UserID) == "" {
		return model.WeixinBindingSummary{}
	}
	return model.WeixinBindingSummary{
		Bound:     true,
		AccountID: strings.TrimSpace(r.AccountID),
		UserID:    strings.TrimSpace(r.UserID),
		BaseURL:   strings.TrimSpace(r.BaseURL),
		BoundAt:   strings.TrimSpace(r.BoundAt),
	}
}

func normalizeWeixinBindingStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "wait":
		return "wait"
	case "scaned":
		return "scaned"
	case "confirmed":
		return "confirmed"
	case "expired":
		return "expired"
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}

func weixinBindingStatusMessage(status, fallback string) string {
	if strings.TrimSpace(fallback) != "" && status != "wait" {
		return strings.TrimSpace(fallback)
	}

	switch status {
	case "scaned":
		return "二维码已扫描，请在微信中确认登录"
	case "confirmed":
		return "微信绑定成功"
	case "expired":
		return "二维码已过期，请重新发起绑定"
	default:
		return "等待微信扫码"
	}
}

func renderWeixinQRCodeDataURL(content string) (string, error) {
	code, err := qr.Encode(content, qr.M)
	if err != nil {
		return "", err
	}
	code.Scale = 8
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(code.PNG()), nil
}
