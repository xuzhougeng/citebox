package model

type AIFigureJob struct {
	ID       int64             `json:"id"`
	PaperID  int64             `json:"paper_id"`
	Scope    string            `json:"scope"`
	Mode     string            `json:"mode"`
	Language string            `json:"language"`
	Status   string            `json:"status"`
	Items    []AIFigureJobItem `json:"items"`
}

type AIFigureJobItem struct {
	FigureID int64  `json:"figure_id"`
	Status   string `json:"status"`
	Attempts int    `json:"attempts"`
	Error    string `json:"error,omitempty"`
}

type AIFigureJobRequest struct {
	PaperID  int64  `json:"paper_id"`
	Scope    string `json:"scope"`
	Mode     string `json:"mode"`
	Language string `json:"language"`
}
