package agent_session

import "time"

type Surface string

const (
	SurfaceWeChat Surface = "wechat"
	SurfaceWeb    Surface = "web"
)

type ConversationKind string

const (
	KindMainWeChat ConversationKind = "main_wechat"
	KindDefaultWeb ConversationKind = "default_web"
	KindAdHoc      ConversationKind = "ad_hoc"
)

type ConversationRef struct {
	Kind ConversationKind
	ID   int64 // used only when Kind == KindAdHoc
}

type SurfaceContext struct {
	CurrentPaperID        int64
	CurrentPaperTitle     string
	CurrentFigureID       int64
	RecentSearchPaperIDs  []int64
	RecentSearchFigureIDs []int64
}

type Input struct {
	Text  string
	Files []InboundAttachment
}

type InboundAttachment struct {
	MIMEType string
	Path     string
}

type Options struct {
	RequireTTS    bool
	MaxChunkRunes int // 0 = use surface default
}

type AgentRequest struct {
	UserID         string
	Surface        Surface
	Conversation   ConversationRef
	SurfaceContext SurfaceContext
	Input          Input
	Options        Options
}

type ChunkKind string

const (
	ChunkText  ChunkKind = "text"
	ChunkImage ChunkKind = "image"
	ChunkVoice ChunkKind = "voice"
)

type OutboundChunk struct {
	Kind          ChunkKind
	Text          string
	ImagePath     string
	VoicePath     string
	IsPlaceholder bool
}

type EvidenceRef struct {
	Index int // citation number rendered as [N]
	Title string
	DOI   string
}

type AgentResponse struct {
	ConversationID     int64
	UserMessageID      int64
	AssistantMessageID int64
	Chunks             []OutboundChunk
	Evidence           []EvidenceRef
	UsedShortcut       bool
	CreatedAt          time.Time
}
