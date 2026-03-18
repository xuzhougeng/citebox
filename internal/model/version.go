package model

type VersionStatus struct {
	CurrentVersion   string `json:"current_version"`
	BuildTime        string `json:"build_time,omitempty"`
	LatestVersion    string `json:"latest_version,omitempty"`
	LatestReleaseURL string `json:"latest_release_url,omitempty"`
	PublishedAt      string `json:"published_at,omitempty"`
	CheckedAt        string `json:"checked_at,omitempty"`
	Status           string `json:"status"`
	IsLatest         bool   `json:"is_latest"`
	HasUpdate        bool   `json:"has_update"`
	Message          string `json:"message"`
}
