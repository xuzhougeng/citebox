package model

// RemoteMCPSettings describes one user-configured Streamable HTTP MCP server.
// OAuth credentials are deliberately stored outside the application database.
type RemoteMCPSettings struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	Enabled    bool   `json:"enabled"`
	AuthMethod string `json:"auth_method"`
}

type RemoteMCPSettingsView struct {
	RemoteMCPSettings
	Authorized bool     `json:"authorized"`
	ToolNames  []string `json:"tool_names,omitempty"`
}

type MCPAuthorizationStart struct {
	FlowID           string `json:"flow_id"`
	AuthorizationURL string `json:"authorization_url"`
}

type MCPAuthorizationStatus struct {
	Status    string   `json:"status"`
	Message   string   `json:"message,omitempty"`
	ToolNames []string `json:"tool_names,omitempty"`
}

type NotionSaveFigureNoteResponse struct {
	Success       bool   `json:"success"`
	Message       string `json:"message"`
	TargetPageID  string `json:"target_page_id"`
	TargetPageURL string `json:"target_page_url,omitempty"`
	ImageEmbedded bool   `json:"image_embedded"`
}
