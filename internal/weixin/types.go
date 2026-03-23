package weixin

const (
	ChannelVersion     = "1.0.2"
	MessageTypeUser    = 1
	MessageTypeBot     = 2
	MessageStateFinish = 2
	ItemTypeText       = 1
	ItemTypeImage      = 2
	ItemTypeVoice      = 3
	ItemTypeFile       = 4
	ItemTypeVideo      = 5
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
	ImageItem *ImageItem `json:"image_item,omitempty"`
	VoiceItem *VoiceItem `json:"voice_item,omitempty"`
	FileItem  *FileItem  `json:"file_item,omitempty"`
	VideoItem *VideoItem `json:"video_item,omitempty"`
}

type TextItem struct {
	Text string `json:"text"`
}

type CDNMedia struct {
	EncryptQueryParam string `json:"encrypt_query_param,omitempty"`
	AESKey            string `json:"aes_key,omitempty"`
	EncryptType       int    `json:"encrypt_type,omitempty"`
}

type ImageItem struct {
	Media   *CDNMedia `json:"media,omitempty"`
	AESKey  string    `json:"aeskey,omitempty"`
	URL     string    `json:"url,omitempty"`
	MidSize int       `json:"mid_size,omitempty"`
}

type VoiceItem struct {
	Media *CDNMedia `json:"media,omitempty"`
	Text  string    `json:"text,omitempty"`
}

type FileItem struct {
	Media    *CDNMedia `json:"media,omitempty"`
	FileName string    `json:"file_name,omitempty"`
	MD5      string    `json:"md5,omitempty"`
	Len      string    `json:"len,omitempty"`
}

type VideoItem struct {
	Media *CDNMedia `json:"media,omitempty"`
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

type GetUploadURLRequest struct {
	FileKey     string   `json:"filekey"`
	MediaType   int      `json:"media_type"`
	ToUserID    string   `json:"to_user_id"`
	RawSize     int      `json:"rawsize"`
	RawFileMD5  string   `json:"rawfilemd5"`
	FileSize    int      `json:"filesize"`
	NoNeedThumb bool     `json:"no_need_thumb"`
	AESKey      string   `json:"aeskey"`
	BaseInfo    BaseInfo `json:"base_info"`
}

type GetUploadURLResponse struct {
	Ret              int    `json:"ret"`
	ErrCode          int    `json:"errcode"`
	Message          string `json:"message"`
	UploadParam      string `json:"upload_param"`
	ThumbUploadParam string `json:"thumb_upload_param"`
}
