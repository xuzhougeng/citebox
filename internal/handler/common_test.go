package handler

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/apperr"
)

func newMultipartRequest(t *testing.T, field, filename string, content []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile(field, filename)
	if err != nil {
		t.Fatalf("CreateFormFile() error = %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("part.Write() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close() error = %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/papers", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if err := req.ParseMultipartForm(1 << 20); err != nil {
		t.Fatalf("ParseMultipartForm() error = %v", err)
	}
	return req
}

func TestFormFileReportsEmptyUploadPart(t *testing.T) {
	req := newMultipartRequest(t, "pdf", "paper.pdf", nil)

	_, _, err := formFile(req, "pdf", "缺少 PDF 文件")
	if err == nil {
		t.Fatal("formFile() error = nil, want empty upload error")
	}
	if !apperr.IsCode(err, apperr.CodeInvalidArgument) {
		t.Fatalf("formFile() code = %v, want invalid argument", apperr.CodeOf(err))
	}
	message := apperr.Message(err)
	if !strings.Contains(message, "0 字节") || !strings.Contains(message, "paper.pdf") {
		t.Fatalf("formFile() message = %q, want filename and byte count", message)
	}
}

func TestFormFileReturnsUploadedPart(t *testing.T) {
	req := newMultipartRequest(t, "pdf", "paper.pdf", []byte("%PDF-1.7"))

	file, header, err := formFile(req, "pdf", "缺少 PDF 文件")
	if err != nil {
		t.Fatalf("formFile() error = %v", err)
	}
	defer file.Close()
	if header.Size != int64(len("%PDF-1.7")) {
		t.Fatalf("header.Size = %d, want %d", header.Size, len("%PDF-1.7"))
	}

	if _, _, err := formFile(req, "missing", "缺少 PDF 文件"); apperr.Message(err) != "缺少 PDF 文件" {
		t.Fatalf("formFile(missing) message = %q, want missing-file message", apperr.Message(err))
	}
}
