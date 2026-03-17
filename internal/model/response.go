package model

type ErrorResponse struct {
	Success bool   `json:"success"`
	Code    string `json:"code"`
	Error   string `json:"error"`
	Paper   *Paper `json:"paper,omitempty"`
}

type SuccessResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}
