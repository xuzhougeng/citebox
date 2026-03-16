package model

type ManualExtractionRegion struct {
	PageNumber      int     `json:"page_number"`
	X               float64 `json:"x"`
	Y               float64 `json:"y"`
	Width           float64 `json:"width"`
	Height          float64 `json:"height"`
	Caption         string  `json:"caption,omitempty"`
	ReplaceFigureID *int64  `json:"replace_figure_id,omitempty"`
}

type ManualExtractionWorkspace struct {
	Paper     *Paper `json:"paper"`
	PageCount int    `json:"page_count"`
}
