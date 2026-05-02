package service

import (
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

