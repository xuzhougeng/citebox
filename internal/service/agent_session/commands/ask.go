package commands

import (
	"context"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

// PaperQAService is the narrow surface /ask and /interpret need from the AI
// service. *service.AIService satisfies it via ReadPaper(...). Defined as an
// interface so the commands subpackage can be tested without instantiating
// the full AI stack.
type PaperQAService interface {
	ReadPaper(ctx context.Context, input model.AIReadRequest) (*model.AIReadResponse, error)
}

// AskCommand routes a question to the constrained paper-Q&A AI path against
// the surface's currently-selected paper.
type AskCommand struct {
	AI PaperQAService
}

func (*AskCommand) Name() string { return "/ask" }

func (c *AskCommand) Execute(ctx context.Context, arg string, rt RuntimeCtx) (*Result, error) {
	q := strings.TrimSpace(arg)
	if q == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "/ask 后请跟问题")
	}
	if rt.State == nil {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "缺少 surface state")
	}
	sc, _ := rt.State.Get(rt.Surface)
	if sc.CurrentPaperID <= 0 {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "请先选定一篇文献再 /ask。")
	}
	resp, err := c.AI.ReadPaper(ctx, model.AIReadRequest{
		PaperID:  sc.CurrentPaperID,
		Action:   model.AIActionPaperQA,
		Question: q,
	})
	if err != nil {
		return nil, err
	}
	answer := ""
	if resp != nil {
		answer = strings.TrimSpace(resp.Answer)
	}
	if answer == "" {
		answer = "（模型未返回正文）"
	}
	return &Result{ChunksText: []string{answer}}, nil
}
