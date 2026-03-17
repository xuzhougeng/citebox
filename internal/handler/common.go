package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/service"
)

func sendJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
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
