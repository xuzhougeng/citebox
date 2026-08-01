package model

// NotionAPISettingsView deliberately omits the personal access token. The
// frontend only needs to know whether a credential has been configured and
// which Notion user it belongs to.
type NotionAPISettingsView struct {
	Configured bool   `json:"configured"`
	UserID     string `json:"user_id,omitempty"`
	UserName   string `json:"user_name,omitempty"`
}

type NotionAPITokenStatus struct {
	Success  bool   `json:"success"`
	Message  string `json:"message"`
	UserID   string `json:"user_id,omitempty"`
	UserName string `json:"user_name,omitempty"`
}
