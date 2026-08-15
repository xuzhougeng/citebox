package zotero

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClientListsCollectionsAndItems(t *testing.T) {
	pdfPath := filepath.Join(t.TempDir(), "paper.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/users/0/collections"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
				{"key":"C1","data":{"key":"C1","name":"Lymph","parentCollection":false}},
				{"key":"C2","data":{"key":"C2","name":"T cell","parentCollection":"C1"}}
			]`))
		case strings.HasSuffix(r.URL.Path, "/users/0/items"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
				{"key":"P1","data":{"key":"P1","itemType":"journalArticle","title":"Atlas","DOI":"10.1000/test","collections":["C2"],"creators":[{"creatorType":"author","firstName":"Ada","lastName":"Lovelace"}],"tags":[{"tag":"sc"}]}},
				{"key":"A1","data":{"key":"A1","itemType":"attachment","parentItem":"P1","contentType":"application/pdf","filename":"paper.pdf","linkMode":"imported_file"}}
			]`))
		case strings.HasSuffix(r.URL.Path, "/users/0/items/A1/file/view/url"):
			_, _ = w.Write([]byte("file://" + pdfPath))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(server.URL + "/api")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	status, err := client.Probe(context.Background())
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if status.LibraryPrefix != "users/0" || status.CollectionCount != 2 {
		t.Fatalf("Probe() = %+v", status)
	}
	items, err := client.ListItems(context.Background(), "users/0")
	if err != nil {
		t.Fatalf("ListItems() error = %v", err)
	}
	if len(items) != 2 || items[0].DOI != "10.1000/test" {
		t.Fatalf("ListItems() = %+v", items)
	}
	gotPath, err := client.AttachmentFilePath(context.Background(), "users/0", "A1")
	if err != nil {
		t.Fatalf("AttachmentFilePath() error = %v", err)
	}
	if gotPath != pdfPath {
		t.Fatalf("AttachmentFilePath() = %q, want %q", gotPath, pdfPath)
	}
}

func TestNewClientRejectsRemoteBaseURL(t *testing.T) {
	if _, err := NewClient("http://93.184.216.34:23119/api"); err == nil {
		t.Fatal("expected remote client to be rejected")
	}
}
