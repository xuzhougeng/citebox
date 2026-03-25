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

type WeixinBridgeSettings struct {
	Enabled bool `json:"enabled"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type AuthSettings struct {
	Username       string               `json:"username"`
	PasswordFromDB bool                 `json:"password_from_db"`
	WeixinBinding  WeixinBindingSummary `json:"weixin_binding"`
	WeixinBridge   WeixinBridgeSettings `json:"weixin_bridge"`
}
