package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net"
	"net/http"
	"strconv"
	"strings"
	"syscall"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/service"
)

func sendJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// formFile returns the uploaded file in field. A part that arrives with no
// bytes gets its own error: it means the client did not send the file
// contents (issue #73), not that the file itself is broken.
func formFile(r *http.Request, field, missingMessage string) (multipart.File, *multipart.FileHeader, error) {
	file, header, err := r.FormFile(field)
	if err != nil {
		return nil, nil, apperr.New(apperr.CodeInvalidArgument, missingMessage)
	}
	if header.Size == 0 {
		file.Close()
		return nil, nil, apperr.New(apperr.CodeInvalidArgument, fmt.Sprintf(
			"上传请求未收到文件内容（「%s」收到 0 字节），文件本身通常没有问题；请重试，或用浏览器打开本地地址上传",
			header.Filename,
		))
	}
	return file, header, nil
}

func sendError(w http.ResponseWriter, err error) {
	status := apperr.HTTPStatus(err)
	resp := model.ErrorResponse{
		Success: false,
		Code:    string(apperr.CodeOf(err)),
		Error:   apperr.Message(err),
	}

	var duplicateErr *service.DuplicatePaperError
	if errors.As(err, &duplicateErr) {
		resp.Paper = duplicateErr.Paper
	}

	sendJSON(w, status, resp)
}

func isClientDisconnectError(err error) bool {
	return errors.Is(err, context.Canceled) ||
		errors.Is(err, net.ErrClosed) ||
		errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ECONNABORTED)
}

func parseIDFromPath(path, prefix string) (int64, error) {
	value := strings.TrimPrefix(path, prefix)
	value = strings.Trim(value, "/")
	return strconv.ParseInt(value, 10, 64)
}

func parseIDWithSuffix(path, prefix, suffix string) (int64, error) {
	value := strings.TrimPrefix(path, prefix)
	value = strings.TrimSuffix(value, suffix)
	value = strings.Trim(value, "/")
	return strconv.ParseInt(value, 10, 64)
}
