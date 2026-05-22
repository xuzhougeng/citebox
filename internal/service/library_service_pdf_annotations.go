package service

import (
	"math"
	"net/url"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
)

type CreatePDFAnnotationParams struct {
	Type      string                        `json:"type"`
	QuoteText string                        `json:"quote_text"`
	Color     string                        `json:"color"`
	Fragments []model.PDFAnnotationFragment `json:"fragments"`
}

type ListPDFAnnotationsParams struct {
	Query    string
	Sort     string
	Page     int
	PageSize int
}

func (s *LibraryService) ListPDFAnnotations(paperID int64) ([]model.PDFAnnotation, error) {
	if _, err := s.GetPaper(paperID); err != nil {
		return nil, err
	}
	return s.repo.PDFAnnotation.ListByPaperID(paperID)
}

func (s *LibraryService) ListPDFAnnotationsGlobal(params ListPDFAnnotationsParams) (*model.PDFAnnotationListResponse, error) {
	filter, err := normalizePDFAnnotationListFilter(params)
	if err != nil {
		return nil, err
	}
	items, total, err := s.repo.PDFAnnotation.ListGlobal(filter)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].PaperStoredPDFName != "" {
			items[i].PaperPDFURL = "/files/papers/" + url.PathEscape(items[i].PaperStoredPDFName)
		}
	}
	totalPages := 0
	if total > 0 {
		totalPages = int(math.Ceil(float64(total) / float64(filter.PageSize)))
	}
	return &model.PDFAnnotationListResponse{
		Annotations: items,
		Pagination: model.PDFAnnotationListPagination{
			Page:       filter.Page,
			PageSize:   filter.PageSize,
			Total:      total,
			TotalPages: totalPages,
		},
	}, nil
}

func (s *LibraryService) CreatePDFAnnotation(paperID int64, params CreatePDFAnnotationParams) (*model.PDFAnnotation, error) {
	if _, err := s.GetPaper(paperID); err != nil {
		return nil, err
	}

	input, err := normalizePDFAnnotationCreateInput(params)
	if err != nil {
		return nil, err
	}
	return s.repo.PDFAnnotation.Create(paperID, input)
}

func (s *LibraryService) DeletePDFAnnotation(paperID, annotationID int64) error {
	if annotationID <= 0 {
		return apperr.New(apperr.CodeInvalidArgument, "pdf annotation id 无效")
	}
	if _, err := s.GetPaper(paperID); err != nil {
		return err
	}
	return s.repo.PDFAnnotation.Delete(paperID, annotationID)
}

func normalizePDFAnnotationListFilter(params ListPDFAnnotationsParams) (repository.PDFAnnotationListFilter, error) {
	sort := strings.TrimSpace(params.Sort)
	if sort == "" {
		sort = "updated_desc"
	}
	switch sort {
	case "updated_desc", "updated_asc", "created_desc", "created_asc":
	default:
		return repository.PDFAnnotationListFilter{}, apperr.New(apperr.CodeInvalidArgument, "PDF 标注排序方式无效")
	}

	page := params.Page
	if page < 1 {
		page = 1
	}
	pageSize := params.PageSize
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 100 {
		pageSize = 100
	}

	return repository.PDFAnnotationListFilter{
		Query:    strings.TrimSpace(params.Query),
		Sort:     sort,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func normalizePDFAnnotationCreateInput(params CreatePDFAnnotationParams) (repository.PDFAnnotationCreateInput, error) {
	annotationType := strings.TrimSpace(params.Type)
	if annotationType == "" {
		annotationType = "highlight"
	}
	if annotationType != "highlight" {
		return repository.PDFAnnotationCreateInput{}, apperr.New(apperr.CodeInvalidArgument, "PDF 标注类型无效")
	}

	color := strings.TrimSpace(params.Color)
	if color == "" {
		color = "yellow"
	}
	if color != "yellow" {
		return repository.PDFAnnotationCreateInput{}, apperr.New(apperr.CodeInvalidArgument, "PDF 高亮颜色无效")
	}

	quoteText := strings.TrimSpace(params.QuoteText)
	if quoteText == "" {
		return repository.PDFAnnotationCreateInput{}, apperr.New(apperr.CodeInvalidArgument, "PDF 高亮文本不能为空")
	}
	if len([]rune(quoteText)) > 10000 {
		return repository.PDFAnnotationCreateInput{}, apperr.New(apperr.CodeInvalidArgument, "PDF 高亮文本过长")
	}

	fragments, err := normalizePDFAnnotationFragments(params.Fragments)
	if err != nil {
		return repository.PDFAnnotationCreateInput{}, err
	}

	return repository.PDFAnnotationCreateInput{
		Type:      annotationType,
		QuoteText: quoteText,
		Color:     color,
		Fragments: fragments,
		NoteText:  "",
	}, nil
}

func normalizePDFAnnotationFragments(fragments []model.PDFAnnotationFragment) ([]model.PDFAnnotationFragment, error) {
	if len(fragments) == 0 {
		return nil, apperr.New(apperr.CodeInvalidArgument, "PDF 高亮位置不能为空")
	}
	if len(fragments) > 200 {
		return nil, apperr.New(apperr.CodeInvalidArgument, "PDF 高亮位置过多")
	}

	normalized := make([]model.PDFAnnotationFragment, 0, len(fragments))
	for _, fragment := range fragments {
		if err := validatePDFAnnotationFragment(fragment); err != nil {
			return nil, err
		}
		normalized = append(normalized, fragment)
	}
	return normalized, nil
}

func validatePDFAnnotationFragment(fragment model.PDFAnnotationFragment) error {
	if fragment.Page < 1 {
		return apperr.New(apperr.CodeInvalidArgument, "PDF 高亮页码必须从 1 开始")
	}
	values := []float64{fragment.Left, fragment.Top, fragment.Width, fragment.Height}
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return apperr.New(apperr.CodeInvalidArgument, "PDF 高亮位置无效")
		}
	}
	if fragment.Left < 0 || fragment.Top < 0 {
		return apperr.New(apperr.CodeInvalidArgument, "PDF 高亮坐标必须在页面内")
	}
	if fragment.Width <= 0 || fragment.Height <= 0 {
		return apperr.New(apperr.CodeInvalidArgument, "PDF 高亮宽高必须大于 0")
	}
	if fragment.Left+fragment.Width > 1.001 || fragment.Top+fragment.Height > 1.001 {
		return apperr.New(apperr.CodeInvalidArgument, "PDF 高亮位置不能超出页面")
	}
	return nil
}
