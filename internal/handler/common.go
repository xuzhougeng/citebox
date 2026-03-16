package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"paper_image_db/internal/model"
)

func sendJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func sendError(w http.ResponseWriter, status int, message string) {
	sendJSON(w, status, model.ErrorResponse{
		Success: false,
		Error:   message,
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
