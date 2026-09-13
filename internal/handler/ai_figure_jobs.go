package handler

import (
	"encoding/json"
	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"net/http"
	"strconv"
	"strings"
)

func (h *AIHandler) FigureJobs(w http.ResponseWriter, r *http.Request) {
	var job *model.AIFigureJob
	var err error
	status := http.StatusOK
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/ai/figure-jobs"), "/")
	if path == "" {
		switch r.Method {
		case http.MethodPost:
			var in model.AIFigureJobRequest
			if decodeErr := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&in); decodeErr != nil {
				sendError(w, apperr.New(apperr.CodeInvalidArgument, "Invalid task request"))
				return
			}
			job, err = h.service.StartFigureAIJob(in)
			status = http.StatusAccepted
		case http.MethodGet:
			paperID, _ := strconv.ParseInt(r.URL.Query().Get("paper_id"), 10, 64)
			if paperID <= 0 {
				sendError(w, apperr.New(apperr.CodeInvalidArgument, "paper_id is required"))
				return
			}
			job, err = h.service.LatestFigureAIJob(paperID)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
	} else {
		parts := strings.Split(path, "/")
		id, _ := strconv.ParseInt(parts[0], 10, 64)
		if id <= 0 || len(parts) > 2 {
			sendError(w, apperr.New(apperr.CodeInvalidArgument, "Invalid task id"))
			return
		}
		if len(parts) == 1 && r.Method == http.MethodGet {
			job, err = h.service.GetFigureAIJob(id)
		} else if len(parts) == 2 && r.Method == http.MethodPost {
			switch parts[1] {
			case "resume":
				job, err = h.service.ResumeFigureAIJob(id)
				status = http.StatusAccepted
			case "stop":
				job, err = h.service.StopFigureAIJob(id)
			default:
				w.WriteHeader(http.StatusNotFound)
				return
			}
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
	}
	if err != nil {
		sendError(w, err)
		return
	}
	sendJSON(w, status, map[string]any{"job": job})
}
