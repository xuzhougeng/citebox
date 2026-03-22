package weixin

// QRCodeResponse is returned by the iLink QR login endpoint.
type QRCodeResponse struct {
	Ret              int    `json:"ret"`
	QRCode           string `json:"qrcode"`
	QRCodeImgContent string `json:"qrcode_img_content"`
	Message          string `json:"message"`
}

// QRCodeStatusResponse is returned while polling the QR login state.
type QRCodeStatusResponse struct {
	Ret         int    `json:"ret"`
	Status      string `json:"status"`
	BotToken    string `json:"bot_token"`
	BaseURL     string `json:"baseurl"`
	ILinkBotID  string `json:"ilink_bot_id"`
	ILinkUserID string `json:"ilink_user_id"`
	Message     string `json:"message"`
}
