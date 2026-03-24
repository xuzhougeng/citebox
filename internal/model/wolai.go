package model

type WolaiSettings struct {
	Token         string `json:"token"`
	ParentBlockID string `json:"parent_block_id"`
	BaseURL       string `json:"base_url"`
}

type WolaiTestResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type WolaiSaveNoteResponse struct {
	Success       bool   `json:"success"`
	Message       string `json:"message"`
	TargetBlockID string `json:"target_block_id"`
}
