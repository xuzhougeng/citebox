package ai_image_gen

import (
	"fmt"
	"strings"
)

type PaperContext struct {
	ID              int64
	Title           string
	Abstract        string
	FullTextSnippet string
	Figures         []FigureContext
}

type FigureContext struct {
	ID          int64
	PaperTitle  string
	PageNumber  int
	FigureIndex int
	Caption     string
}

type VisionInputContext struct {
	UserText        string
	Papers          []PaperContext
	IsolatedFigures []FigureContext
}

const visionSystemPrompt = `You are an experienced graphical-abstract designer for scientific papers. Your job: read the provided paper text and figures, then write a concise, vivid, well-structured English prompt (300–600 tokens) that an image generation model can render into ONE 1024x1024 graphical abstract.

Rules:
- Output ONLY the image prompt text. No prose explanation, no numbered list, no headers.
- Use clear visual nouns: "left panel", "central diagram", "icons of...", "arrows showing...".
- Keep typography minimal — at most 2–3 short labels in English.
- Convey the paper's core finding, not every figure.
- Avoid copyrighted character references and photorealistic faces.`

func VisionSystemPrompt() string { return visionSystemPrompt }

func BuildVisionUserPrompt(ctx VisionInputContext) string {
	var b strings.Builder
	b.WriteString("用户的请求：\n")
	b.WriteString(strings.TrimSpace(ctx.UserText))
	b.WriteString("\n\n")

	for _, p := range ctx.Papers {
		b.WriteString(fmt.Sprintf("=== 文献 (id=%d) ===\n", p.ID))
		b.WriteString("标题：")
		b.WriteString(strings.TrimSpace(p.Title))
		b.WriteString("\n")
		if abstract := strings.TrimSpace(p.Abstract); abstract != "" {
			b.WriteString("摘要：")
			b.WriteString(abstract)
			b.WriteString("\n")
		}
		if snippet := strings.TrimSpace(p.FullTextSnippet); snippet != "" {
			b.WriteString("正文片段：")
			b.WriteString(snippet)
			b.WriteString("\n")
		}
		for _, fig := range p.Figures {
			b.WriteString(figureLine(fig))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if len(ctx.IsolatedFigures) > 0 {
		b.WriteString("=== 用户单独引用的图 ===\n")
		for _, fig := range ctx.IsolatedFigures {
			b.WriteString(figureLine(fig))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("请基于上述材料，输出一段图像生成 prompt。")
	return b.String()
}

func figureLine(fig FigureContext) string {
	caption := strings.TrimSpace(fig.Caption)
	if caption == "" {
		caption = "（无描述）"
	}
	return fmt.Sprintf("- 第 %d 页图 %d（id=%d，来自《%s》）：%s",
		fig.PageNumber, fig.FigureIndex, fig.ID, strings.TrimSpace(fig.PaperTitle), caption)
}
