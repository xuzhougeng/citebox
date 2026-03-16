package model

type ExtractorSettings struct {
	ExtractorURL          string `json:"extractor_url"`
	ExtractorJobsURL      string `json:"extractor_jobs_url"`
	ExtractorToken        string `json:"extractor_token"`
	ExtractorFileField    string `json:"extractor_file_field"`
	TimeoutSeconds        int    `json:"timeout_seconds"`
	PollIntervalSeconds   int    `json:"poll_interval_seconds"`
	EffectiveExtractorURL string `json:"effective_extractor_url"`
	EffectiveJobsURL      string `json:"effective_jobs_url"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type AuthSettings struct {
	Username        string `json:"username"`
	PasswordFromDB  bool   `json:"password_from_db"`
}
