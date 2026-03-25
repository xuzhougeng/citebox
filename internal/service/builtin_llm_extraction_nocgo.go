//go:build !cgo || nocgo

package service

import (
	"context"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

func builtInLLMUnavailableError() error {
	return apperr.New(apperr.CodeUnavailable, "当前构建未启用 cgo，内置 PDF 渲染与全文提取不可用")
}

func (s *LibraryService) processBuiltInLLMExtraction(settings model.ExtractorSettings, paperID int64, pdfPath, originalFilename string) error {
	return builtInLLMUnavailableError()
}

func (s *LibraryService) extractBuiltInLLMResult(ctx context.Context, paperID int64, pdfPath, originalFilename string) (*extractionResult, error) {
	return nil, builtInLLMUnavailableError()
}

func (s *LibraryService) extractServerPDFTextFallback(pdfPath string) (string, error) {
	return "", builtInLLMUnavailableError()
}
