package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

func sendJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func sendError(w http.ResponseWriter, err error) {
	status := apperr.HTTPStatus(err)
	sendJSON(w, status, model.ErrorResponse{
		Success: false,
		Code:    string(apperr.CodeOf(err)),
		Error:   apperr.Message(err),
	})
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
