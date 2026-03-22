package weixin

const (
	ChannelVersion     = "1.0.2"
	MessageTypeUser    = 1
	MessageTypeBot     = 2
	MessageStateFinish = 2
	ItemTypeText       = 1
	ItemTypeVoice      = 3
)

// BaseInfo is required by the iLink protocol in every POST request body.
type BaseInfo struct {
	ChannelVersion string `json:"channel_version"`
}

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

type GetUpdatesRequest struct {
	GetUpdatesBuf string   `json:"get_updates_buf"`
	BaseInfo      BaseInfo `json:"base_info"`
}

type GetUpdatesResponse struct {
	Ret                int       `json:"ret"`
	Msgs               []Message `json:"msgs"`
	GetUpdatesBuf      string    `json:"get_updates_buf"`
	LongPollingTimeout int       `json:"longpolling_timeout_ms"`
	ErrCode            int       `json:"errcode"`
}

type Message struct {
	FromUserID   string        `json:"from_user_id"`
	ToUserID     string        `json:"to_user_id"`
	ClientID     string        `json:"client_id,omitempty"`
	MessageType  int           `json:"message_type"`
	MessageState int           `json:"message_state"`
	ContextToken string        `json:"context_token"`
	ItemList     []MessageItem `json:"item_list"`
	GroupID      string        `json:"group_id,omitempty"`
}

type MessageItem struct {
	Type      int        `json:"type"`
	TextItem  *TextItem  `json:"text_item,omitempty"`
	VoiceItem *VoiceItem `json:"voice_item,omitempty"`
}

type TextItem struct {
	Text string `json:"text"`
}

type VoiceItem struct {
	Text string `json:"text,omitempty"`
}

type SendMessageRequest struct {
	Msg      Message  `json:"msg"`
	BaseInfo BaseInfo `json:"base_info"`
}

type SendMessageResponse struct {
	Ret     int    `json:"ret"`
	ErrCode int    `json:"errcode"`
	Message string `json:"message"`
}
