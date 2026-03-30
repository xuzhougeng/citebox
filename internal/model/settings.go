package model

type ExtractorSettings struct {
	ExtractorProfile      string `json:"extractor_profile"`
	PDFTextSource         string `json:"pdf_text_source"`
	ExtractorURL          string `json:"extractor_url"`
	ExtractorJobsURL      string `json:"extractor_jobs_url"`
	ExtractorToken        string `json:"extractor_token"`
	ExtractorFileField    string `json:"extractor_file_field"`
	TimeoutSeconds        int    `json:"timeout_seconds"`
	PollIntervalSeconds   int    `json:"poll_interval_seconds"`
	EffectiveExtractorURL string `json:"effective_extractor_url"`
	EffectiveJobsURL      string `json:"effective_jobs_url"`
}

const (
	DesktopCloseActionAsk      = "ask"
	DesktopCloseActionMinimize = "minimize"
	DesktopCloseActionExit     = "exit"
)

type DesktopCloseSettings struct {
	Action string `json:"action"`
}

func NormalizeDesktopCloseAction(action string) string {
	switch action {
	case DesktopCloseActionMinimize, DesktopCloseActionExit:
		return action
	default:
		return DesktopCloseActionAsk
	}
}

type WeixinBridgeSettings struct {
	Enabled             bool                              `json:"enabled"`
	DailyRecommendation WeixinDailyRecommendationSettings `json:"daily_recommendation"`
}

type WeixinDailyRecommendationSettings struct {
	Enabled  bool   `json:"enabled"`
	SendTime string `json:"send_time"`
}

type TTSSettings struct {
	AppID                       string `json:"app_id"`
	AccessKey                   string `json:"access_key"`
	ResourceID                  string `json:"resource_id"`
	Speaker                     string `json:"speaker"`
	WeixinVoiceOutputEnabled    bool   `json:"weixin_voice_output_enabled"`
	WeixinVoiceOutputEnabledSet bool   `json:"-"`
}

const DefaultWeixinVoiceOutputEnabled = true
const DefaultWeixinDailyRecommendationSendTime = "09:00"

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type AuthSettings struct {
	Username             string               `json:"username"`
	PasswordFromDB       bool                 `json:"password_from_db"`
	RememberLoginEnabled bool                 `json:"remember_login_enabled"`
	WeixinBinding        WeixinBindingSummary `json:"weixin_binding"`
	WeixinBridge         WeixinBridgeSettings `json:"weixin_bridge"`
}
