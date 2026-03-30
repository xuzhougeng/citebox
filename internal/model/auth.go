package model

type LoginRequest struct {
	Username      string `json:"username"`
	Password      string `json:"password"`
	RememberLogin bool   `json:"remember_login"`
}

type RememberLoginRequest struct {
	Enabled bool `json:"enabled"`
}

type WeixinBindingSummary struct {
	Bound     bool   `json:"bound"`
	AccountID string `json:"account_id,omitempty"`
	UserID    string `json:"user_id,omitempty"`
	BaseURL   string `json:"base_url,omitempty"`
	BoundAt   string `json:"bound_at,omitempty"`
}

type WeixinBindingStartResponse struct {
	QRCode        string `json:"qrcode"`
	QRCodeContent string `json:"qrcode_content"`
	QRCodeDataURL string `json:"qrcode_data_url"`
	Status        string `json:"status"`
	Message       string `json:"message,omitempty"`
}

type WeixinBindingStatusResponse struct {
	Status  string               `json:"status"`
	Message string               `json:"message,omitempty"`
	Binding WeixinBindingSummary `json:"binding"`
}
