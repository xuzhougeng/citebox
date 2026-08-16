package model

type FigureLibrarySettings struct {
	DropDir string `json:"drop_dir"`
}

type FigureLibraryStatus struct {
	Configured bool   `json:"configured"`
	Ready      bool   `json:"ready"`
	DropDir    string `json:"drop_dir"`
	Message    string `json:"message"`
}

type FigureLibrarySendResult struct {
	FigureID int64  `json:"figure_id"`
	Filename string `json:"filename"`
	Path     string `json:"path"`
}
