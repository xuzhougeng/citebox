package mcpserver

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/integration"
	"github.com/xuzhougeng/citebox/internal/model"
)

type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// callTool 执行 tools/call：工具存在性 → 权限范围 → 参数解码校验 → 调用门面
func (s *Server) callTool(rawParams json.RawMessage, token *model.IntegrationToken) (any, *rpcError) {
	var params toolCallParams
	if err := json.Unmarshal(rawParams, &params); err != nil {
		return nil, &rpcError{Code: rpcCodeInvalidParams, Message: "Invalid params"}
	}
	name := strings.TrimSpace(params.Name)
	if !knownTool(name) {
		return nil, &rpcError{Code: rpcCodeMethodNotFound, Message: "Unknown tool: " + name}
	}
	if scope := requiredScope(name, params.Arguments); scope != "" && !integration.HasScope(token, scope) {
		return nil, &rpcError{Code: rpcCodeInvalidRequest, Message: "insufficient scope: " + scope + " required"}
	}
	return s.executeTool(name, params.Arguments)
}

func knownTool(name string) bool {
	for _, tool := range integration.ToolNames() {
		if tool == name {
			return true
		}
	}
	return false
}

// requiredScope 返回工具调用所需的权限范围；空串表示任何有效令牌均可
func requiredScope(name string, arguments json.RawMessage) string {
	switch name {
	case integration.ToolGetCapabilities:
		return ""
	case integration.ToolExportAsset:
		return integration.ScopeAssetsRead
	case integration.ToolGetEntity:
		// 按 source_id 指向的实体类型决定权限范围
		var args struct {
			SourceID string `json:"source_id"`
		}
		if err := json.Unmarshal(arguments, &args); err == nil {
			if ref, err := integration.ParseSourceID(args.SourceID); err == nil {
				switch ref.Kind {
				case integration.EntityTypeNote:
					return integration.ScopeNotesRead
				case integration.EntityTypeAnnotation:
					return integration.ScopeAnnotationsRead
				}
			}
		}
		return integration.ScopeLibraryRead
	default:
		return integration.ScopeLibraryRead
	}
}

// decodeArguments 解码工具参数；arguments 缺省时保留零值
func decodeArguments(raw json.RawMessage, target any) *rpcError {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return &rpcError{Code: rpcCodeInvalidParams, Message: "Invalid params: " + err.Error()}
	}
	return nil
}

func invalidParams(message string) *rpcError {
	return &rpcError{Code: rpcCodeInvalidParams, Message: "Invalid params: " + message}
}

func (s *Server) executeTool(name string, arguments json.RawMessage) (any, *rpcError) {
	switch name {
	case integration.ToolGetCapabilities:
		return toolSuccessResult(s.facade.GetCapabilities())
	case integration.ToolSearchLibrary:
		var args struct {
			Query        string   `json:"query"`
			EntityTypes  []string `json:"entity_types"`
			GroupID      *int64   `json:"group_id"`
			Tags         []int64  `json:"tags"`
			UpdatedAfter string   `json:"updated_after"`
			Cursor       string   `json:"cursor"`
			Limit        int      `json:"limit"`
		}
		if rpcErr := decodeArguments(arguments, &args); rpcErr != nil {
			return nil, rpcErr
		}
		params := integration.SearchLibraryParams{
			Query:       args.Query,
			EntityTypes: args.EntityTypes,
			GroupID:     args.GroupID,
			Tags:        args.Tags,
			Cursor:      args.Cursor,
			Limit:       args.Limit,
		}
		if trimmed := strings.TrimSpace(args.UpdatedAfter); trimmed != "" {
			parsed, err := time.Parse(time.RFC3339, trimmed)
			if err != nil {
				return nil, invalidParams("updated_after must be RFC3339")
			}
			params.UpdatedAfter = &parsed
		}
		result, err := s.facade.SearchLibrary(params)
		return toolDomainResult(result, err)
	case integration.ToolGetPaperContext:
		var args struct {
			PaperID         int64    `json:"paper_id"`
			Include         []string `json:"include"`
			FigureLimit     int      `json:"figure_limit"`
			AnnotationLimit int      `json:"annotation_limit"`
		}
		if rpcErr := decodeArguments(arguments, &args); rpcErr != nil {
			return nil, rpcErr
		}
		if args.PaperID <= 0 {
			return nil, invalidParams("paper_id is required")
		}
		result, err := s.facade.GetPaperContext(integration.GetPaperContextParams{
			PaperID:         args.PaperID,
			Include:         args.Include,
			FigureLimit:     args.FigureLimit,
			AnnotationLimit: args.AnnotationLimit,
		})
		return toolDomainResult(result, err)
	case integration.ToolGetFigureHandoff:
		var args struct {
			FigureID int64  `json:"figure_id"`
			SourceID string `json:"source_id"`
		}
		if rpcErr := decodeArguments(arguments, &args); rpcErr != nil {
			return nil, rpcErr
		}
		if args.FigureID <= 0 && strings.TrimSpace(args.SourceID) == "" {
			return nil, invalidParams("figure_id or source_id is required")
		}
		result, err := s.facade.GetFigureHandoff(integration.GetFigureHandoffParams{
			FigureID: args.FigureID,
			SourceID: args.SourceID,
		})
		return toolDomainResult(result, err)
	case integration.ToolSearchPaperText:
		var args struct {
			PaperIDs     []int64 `json:"paper_ids"`
			Query        string  `json:"query"`
			Limit        int     `json:"limit"`
			ContextChars int     `json:"context_chars"`
		}
		if rpcErr := decodeArguments(arguments, &args); rpcErr != nil {
			return nil, rpcErr
		}
		if len(args.PaperIDs) == 0 {
			return nil, invalidParams("paper_ids is required")
		}
		if strings.TrimSpace(args.Query) == "" {
			return nil, invalidParams("query is required")
		}
		result, err := s.facade.SearchPaperText(integration.SearchPaperTextParams{
			PaperIDs:     args.PaperIDs,
			Query:        args.Query,
			Limit:        args.Limit,
			ContextChars: args.ContextChars,
		})
		return toolDomainResult(result, err)
	case integration.ToolGetEntity:
		var args struct {
			SourceID string `json:"source_id"`
		}
		if rpcErr := decodeArguments(arguments, &args); rpcErr != nil {
			return nil, rpcErr
		}
		if strings.TrimSpace(args.SourceID) == "" {
			return nil, invalidParams("source_id is required")
		}
		result, err := s.facade.GetEntity(args.SourceID)
		return toolDomainResult(result, err)
	case integration.ToolExportAsset:
		var args struct {
			Kind string `json:"kind"`
			ID   int64  `json:"id"`
		}
		if rpcErr := decodeArguments(arguments, &args); rpcErr != nil {
			return nil, rpcErr
		}
		switch strings.TrimSpace(args.Kind) {
		case integration.AssetKindFigureImage, integration.AssetKindFigureTransferPackage:
		default:
			return nil, invalidParams("kind must be figure_image or figure_transfer_package")
		}
		if args.ID <= 0 {
			return nil, invalidParams("id is required")
		}
		result, err := s.facade.ExportAsset(args.Kind, args.ID)
		return toolDomainResult(result, err)
	case integration.ToolListChanges:
		var args struct {
			Cursor      string   `json:"cursor"`
			EntityTypes []string `json:"entity_types"`
			Limit       int      `json:"limit"`
		}
		if rpcErr := decodeArguments(arguments, &args); rpcErr != nil {
			return nil, rpcErr
		}
		result, err := s.facade.ListChanges(integration.ListChangesParams{
			Cursor:      args.Cursor,
			EntityTypes: args.EntityTypes,
			Limit:       args.Limit,
		})
		return toolDomainResult(result, err)
	default:
		return nil, &rpcError{Code: rpcCodeMethodNotFound, Message: "Unknown tool: " + name}
	}
}

// toolDomainResult 把门面调用结果映射为 MCP 工具结果：
// 领域错误（未找到/参数无效/前置条件不满足）返回 isError 结果，内部错误返回 JSON-RPC 错误
func toolDomainResult(result any, err error) (any, *rpcError) {
	if err == nil {
		return toolSuccessResult(result)
	}
	switch {
	case apperr.IsCode(err, apperr.CodeNotFound),
		apperr.IsCode(err, apperr.CodeInvalidArgument),
		apperr.IsCode(err, apperr.CodeFailedPrecondition):
		return map[string]any{
			"isError": true,
			"content": []map[string]any{{"type": "text", "text": apperr.Message(err)}},
		}, nil
	default:
		return nil, &rpcError{Code: rpcCodeInternal, Message: "Internal error"}
	}
}

// toolSuccessResult 包装成功结果：content 为格式化 JSON 文本，structuredContent 为结构化数据
func toolSuccessResult(result any) (any, *rpcError) {
	pretty, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, &rpcError{Code: rpcCodeInternal, Message: "Internal error"}
	}
	return map[string]any{
		"content":           []map[string]any{{"type": "text", "text": string(pretty)}},
		"structuredContent": result,
	}, nil
}

// toolDefinitions 返回 tools/list 的工具描述（name/description/inputSchema）
func toolDefinitions() []map[string]any {
	entityTypes := []string{
		integration.EntityTypePaper,
		integration.EntityTypeFigure,
		integration.EntityTypeNote,
		integration.EntityTypeAnnotation,
	}
	return []map[string]any{
		{
			"name":        integration.ToolGetCapabilities,
			"description": "Describe this CiteBox integration: version, schemas, entity types, scopes, limits and the available tools.",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			"name":        integration.ToolSearchLibrary,
			"description": "Search the CiteBox library across papers, figures, notes and PDF annotations. Results merge in fixed type order with cursor paging.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":         map[string]any{"type": "string", "description": "Full-text keyword"},
					"entity_types":  map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": entityTypes}, "description": "Entity types to search; defaults to all"},
					"group_id":      map[string]any{"type": "integer", "description": "Restrict papers/figures to a group"},
					"tags":          map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": "Tag IDs; currently only the first tag is applied"},
					"updated_after": map[string]any{"type": "string", "format": "date-time", "description": "RFC3339; post-filtered per batch"},
					"cursor":        map[string]any{"type": "string", "description": "Opaque cursor from a previous response"},
					"limit":         map[string]any{"type": "integer", "description": "Max items (default 20, max 100)"},
				},
			},
		},
		{
			"name":        integration.ToolGetPaperContext,
			"description": "Build a research-context envelope for one paper: metadata, abstract, notes, figures, annotations, tags and group, trimmed via include flags.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"paper_id":         map[string]any{"type": "integer", "description": "Paper ID"},
					"include":          map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": []string{"metadata", "abstract", "paper_notes", "figure_notes", "annotations", "figures", "tags", "group"}}, "description": "Sections to include; empty means all"},
					"figure_limit":     map[string]any{"type": "integer", "description": "Max figures (default 20)"},
					"annotation_limit": map[string]any{"type": "integer", "description": "Max annotations (default 50)"},
				},
				"required": []string{"paper_id"},
			},
		},
		{
			"name":        integration.ToolGetFigureHandoff,
			"description": "Build a figure-centric research handoff envelope: selected figure or subfigure, paper metadata/abstract, this figure's note, limited figure-anchored excerpts, and asset pointers. Does not generate a biological question or plotting code.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"figure_id": map[string]any{"type": "integer", "description": "Figure ID"},
					"source_id": map[string]any{"type": "string", "description": "citebox:figure:{id}; may replace figure_id"},
				},
			},
		},
		{
			"name":        integration.ToolSearchPaperText,
			"description": "Case-insensitive substring search inside the PDF full text of given papers, returning rune-safe context windows with 1-based page numbers when per-page text exists.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"paper_ids":     map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": "Papers to search"},
					"query":         map[string]any{"type": "string", "description": "Substring to find"},
					"limit":         map[string]any{"type": "integer", "description": "Max matches (default 12)"},
					"context_chars": map[string]any{"type": "integer", "description": "Context window size in runes (default 1200)"},
				},
				"required": []string{"paper_ids", "query"},
			},
		},
		{
			"name":        integration.ToolGetEntity,
			"description": "Fetch one entity envelope by source_id, e.g. citebox:paper:42, citebox:figure:123, citebox:annotation:91 or citebox:note:paper:42:main.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"source_id": map[string]any{"type": "string", "description": "citebox:<kind>:<id> source identifier"},
				},
				"required": []string{"source_id"},
			},
		},
		{
			"name":        integration.ToolExportAsset,
			"description": "Export a binary asset (figure image or figure transfer package) and return a short-lived loopback URL with sha256 for verification.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind": map[string]any{"type": "string", "enum": []string{integration.AssetKindFigureImage, integration.AssetKindFigureTransferPackage}, "description": "Asset kind"},
					"id":   map[string]any{"type": "integer", "description": "Figure ID"},
				},
				"required": []string{"kind", "id"},
			},
		},
		{
			"name":        integration.ToolListChanges,
			"description": "Incrementally sync library changes since a per-type watermark cursor. Note changes ride on parent paper/figure rows.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"cursor":       map[string]any{"type": "string", "description": "Opaque cursor from a previous response; empty for a full scan"},
					"entity_types": map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": entityTypes}, "description": "Entity types to track; defaults to paper, figure and annotation"},
					"limit":        map[string]any{"type": "integer", "description": "Max changes (default 100, max 500)"},
				},
			},
		},
	}
}
