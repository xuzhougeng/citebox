package model

import "time"

const ExternalSourceZotero = "zotero"

type ZoteroSettings struct {
	BaseURL            string   `json:"base_url"`
	IncludeChildren    bool     `json:"include_children"`
	LastCollectionKeys []string `json:"last_collection_keys,omitempty"`
	LastRunID          string   `json:"last_run_id,omitempty"`
}

type ZoteroStatus struct {
	Reachable       bool   `json:"reachable"`
	BaseURL         string `json:"base_url"`
	LibraryPrefix   string `json:"library_prefix,omitempty"`
	CollectionCount int    `json:"collection_count"`
	Message         string `json:"message,omitempty"`
}

type ZoteroCollectionNode struct {
	Key       string                 `json:"key"`
	Name      string                 `json:"name"`
	ParentKey string                 `json:"parent_key,omitempty"`
	Path      string                 `json:"path"`
	Children  []ZoteroCollectionNode `json:"children,omitempty"`
}

type ZoteroImportItem struct {
	ItemKey        string   `json:"item_key"`
	LibraryID      string   `json:"library_id"`
	Title          string   `json:"title"`
	DOI            string   `json:"doi,omitempty"`
	AuthorsText    string   `json:"authors_text,omitempty"`
	Journal        string   `json:"journal,omitempty"`
	PublishedAt    string   `json:"published_at,omitempty"`
	AbstractText   string   `json:"abstract_text,omitempty"`
	NotesText      string   `json:"notes_text,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	CollectionPath string   `json:"collection_path,omitempty"`
	AttachmentKey  string   `json:"attachment_key,omitempty"`
	PDFFilename    string   `json:"pdf_filename,omitempty"`
	PDFPath        string   `json:"pdf_path,omitempty"`
	HasLocalPDF    bool     `json:"has_local_pdf"`
	Status         string   `json:"status"`
	Reason         string   `json:"reason,omitempty"`
	PaperID        int64    `json:"paper_id,omitempty"`
}

type ZoteroImportSummary struct {
	Total           int `json:"total"`
	Imported        int `json:"imported"`
	SkippedExisting int `json:"skipped_existing"`
	MissingPDF      int `json:"missing_pdf"`
	Error           int `json:"error"`
}

type ZoteroImportRun struct {
	ID              string              `json:"id"`
	Status          string              `json:"status"`
	IncludeChildren bool                `json:"include_children"`
	CollectionKeys  []string            `json:"collection_keys"`
	Summary         ZoteroImportSummary `json:"summary"`
	Items           []ZoteroImportItem  `json:"items"`
	ErrorText       string              `json:"error_text,omitempty"`
	CreatedAt       time.Time           `json:"created_at"`
	UpdatedAt       time.Time           `json:"updated_at"`
}

type PaperExternalID struct {
	ID             int64     `json:"id"`
	PaperID        int64     `json:"paper_id"`
	Source         string    `json:"source"`
	LibraryID      string    `json:"library_id"`
	ItemKey        string    `json:"item_key"`
	CollectionPath string    `json:"collection_path,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type ZoteroIngestInput struct {
	ItemKey        string
	LibraryID      string
	Title          string
	DOI            string
	AuthorsText    string
	Journal        string
	PublishedAt    string
	AbstractText   string
	NotesText      string
	Tags           []string
	CollectionPath string
	ExtractionMode string
}
