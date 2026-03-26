package service

import (
	"context"
	"strings"
	"testing"
)

func TestRenderWeixinMarkdownTransformsCommonBlocks(t *testing.T) {
	input := strings.TrimSpace(`
# Overview

This is **bold** and *italic*.

- first item
1. ordered item

> quoted line

Use [docs](https://example.com) and ![Figure 1](figure://123).

` + "```go" + `
fmt.Println("hi")
` + "```" + `

| col1 | col2 |
| --- | --- |
| 1 | 2 |
`)

	got := renderWeixinMarkdown(input)
	if !containsAll(
		got,
		"【一级标题】 Overview",
		"𝗯𝗼𝗹𝗱",
		"𝘪𝘵𝘢𝘭𝘪𝘤",
		"• first item",
		"1. ordered item",
		"│ quoted line",
		"docs (https://example.com)",
		"【图片】Figure 1",
		"─── go ───",
		"    fmt.Println(\"hi\")",
		"| col1 | col2 |",
		"| --- | --- |",
		"| 1 | 2 |",
	) {
		t.Fatalf("renderWeixinMarkdown() = %q, want converted markdown blocks with raw table preserved", got)
	}
}

func TestWeixinIMBridgeAskReplyRendersMarkdownForWeixin(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	createBridgePaper(t, repo, "Markdown Reply Paper", "markdown-reply.pdf")
	bridge := newTestWeixinBridge(t, svc, &fakeWeixinAIReader{
		answer: strings.TrimSpace(`
# Summary

- **bold** point

| name | value |
| --- | --- |
| A | 1 |
`),
	}, cfg.StorageDir)

	_ = bridge.handleIncomingText(context.Background(), "/search Markdown Reply Paper")
	reply := bridge.handleIncomingText(context.Background(), "/ask 总结一下")

	if !containsAll(reply, "文献问答", "【一级标题】 Summary", "• 𝗯𝗼𝗹𝗱 point", "| name | value |", "| A | 1 |") {
		t.Fatalf("handleIncomingText() = %q, want rendered markdown reply with raw table", reply)
	}
	if strings.Contains(reply, "**bold**") {
		t.Fatalf("handleIncomingText() = %q, want markdown emphasis rendered for WeChat", reply)
	}
}
