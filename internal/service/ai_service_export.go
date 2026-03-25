package service

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

var aiMarkdownFigureReferencePattern = regexp.MustCompile(`!\[([^\]]*)\]\(figure://([0-9]+)\)`)

func (s *AIService) ExportReadMarkdown(ctx context.Context, input model.AIReadExportRequest) (string, []byte, error) {
	if input.PaperID <= 0 {
		return "", nil, apperr.New(apperr.CodeInvalidArgument, "paper_id 无效")
	}
	content := strings.TrimSpace(input.Content)
	if content == "" {
		content = strings.TrimSpace(input.Answer)
	}
	if content == "" {
		return "", nil, apperr.New(apperr.CodeInvalidArgument, "缺少可导出的 Markdown 内容")
	}
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}
	scope := normalizeAIReadExportScope(input.Scope)

	paper, err := s.repo.GetPaperDetail(input.PaperID)
	if err != nil {
		return "", nil, err
	}
	if paper == nil {
		return "", nil, apperr.New(apperr.CodeNotFound, "文献不存在")
	}

	figureByID := make(map[int64]model.Figure, len(paper.Figures))
	for _, figure := range paper.Figures {
		figureByID[figure.ID] = figure
	}

	type markdownAsset struct {
		Path string
		Data []byte
	}

	assetPaths := map[int64]string{}
	assets := make([]markdownAsset, 0, 4)
	var rewriteErr error
	rewritten := aiMarkdownFigureReferencePattern.ReplaceAllStringFunc(content, func(match string) string {
		if rewriteErr != nil {
			return match
		}

		parts := aiMarkdownFigureReferencePattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}

		figureID, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			rewriteErr = apperr.New(apperr.CodeInvalidArgument, "回答里的图片引用格式无效")
			return match
		}

		assetPath, ok := assetPaths[figureID]
		if !ok {
			figure, exists := figureByID[figureID]
			if !exists {
				rewriteErr = apperr.New(apperr.CodeInvalidArgument, fmt.Sprintf("回答里引用了当前文献不存在的图片 #%d", figureID))
				return match
			}

			var assetData []byte
			assetPath, assetData, err = s.loadMarkdownExportAsset(paper, figure)
			if err != nil {
				rewriteErr = err
				return match
			}

			assetPaths[figureID] = assetPath
			assets = append(assets, markdownAsset{
				Path: assetPath,
				Data: assetData,
			})
		}

		return fmt.Sprintf("![%s](%s)", parts[1], assetPath)
	})
	if rewriteErr != nil {
		return "", nil, rewriteErr
	}

	var archive bytes.Buffer
	zipWriter := zip.NewWriter(&archive)

	answerWriter, err := zipWriter.Create(aiReadExportMarkdownFilename(scope))
	if err != nil {
		return "", nil, apperr.Wrap(apperr.CodeInternal, "创建 Markdown 导出文件失败", err)
	}
	if _, err := io.WriteString(answerWriter, rewritten); err != nil {
		return "", nil, apperr.Wrap(apperr.CodeInternal, "写入 Markdown 导出内容失败", err)
	}

	for _, asset := range assets {
		fileWriter, err := zipWriter.Create(asset.Path)
		if err != nil {
			return "", nil, apperr.Wrap(apperr.CodeInternal, "创建导出图片文件失败", err)
		}
		if _, err := fileWriter.Write(asset.Data); err != nil {
			return "", nil, apperr.Wrap(apperr.CodeInternal, "写入导出图片文件失败", err)
		}
	}

	if err := zipWriter.Close(); err != nil {
		return "", nil, apperr.Wrap(apperr.CodeInternal, "生成导出压缩包失败", err)
	}

	return aiReadExportFilename(paper, scope, input.TurnIndex), archive.Bytes(), nil
}

func (s *AIService) loadMarkdownExportAsset(paper *model.Paper, figure model.Figure) (string, []byte, error) {
	if paper == nil {
		return "", nil, apperr.New(apperr.CodeNotFound, "paper not found")
	}

	paperFigure := findFigureByID(paper.Figures, figure.ID)
	if paperFigure == nil {
		return "", nil, apperr.New(apperr.CodeNotFound, fmt.Sprintf("导出失败：图片不存在（figure #%d）", figure.ID))
	}

	data, _, err := loadFigureImageData(s.config.FiguresDir(), paper.Figures, *paperFigure)
	if err != nil {
		return "", nil, err
	}

	return "assets/" + aiReadExportAssetName(figure), data, nil
}

func normalizeAIReadExportScope(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "conversation") {
		return "conversation"
	}
	return "turn"
}

func aiReadExportFilename(paper *model.Paper, scope string, turnIndex int) string {
	base := fmt.Sprintf("paper_%d_ai_reader", paper.ID)
	if scope == "conversation" {
		return base + "_conversation.zip"
	}
	if turnIndex > 0 {
		return fmt.Sprintf("%s_turn_%02d.zip", base, turnIndex)
	}
	return base + ".zip"
}

func aiReadExportMarkdownFilename(scope string) string {
	if scope == "conversation" {
		return "conversation.md"
	}
	return "answer.md"
}

func aiReadExportAssetName(figure model.Figure) string {
	ext := extensionForFigure(figure.ContentType, figure.Filename)
	return fmt.Sprintf("figure-p%d-n%d-%d%s", figure.PageNumber, figure.FigureIndex, figure.ID, ext)
}
