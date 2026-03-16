package handler

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"paper_image_db/internal/apperr"
	"paper_image_db/internal/service"
)

type DatabaseHandler struct {
	service *service.LibraryService
}

func NewDatabaseHandler(svc *service.LibraryService) *DatabaseHandler {
	return &DatabaseHandler{service: svc}
}

func (h *DatabaseHandler) Export(w http.ResponseWriter, r *http.Request) {
	dbPath := h.service.DatabasePath()

	file, err := os.Open(dbPath)
	if err != nil {
		sendError(w, apperr.Wrap(apperr.CodeInternal, "无法打开数据库文件", err))
		return
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		sendError(w, apperr.Wrap(apperr.CodeInternal, "无法获取数据库文件信息", err))
		return
	}

	filename := fmt.Sprintf("library_backup_%s.db", time.Now().Format("20060102_150405"))

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", stat.Size()))

	if _, err := io.Copy(w, file); err != nil {
		sendError(w, apperr.Wrap(apperr.CodeInternal, "导出数据库失败", err))
		return
	}
}

func (h *DatabaseHandler) Import(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(512 << 20); err != nil {
		sendError(w, apperr.Wrap(apperr.CodeInvalidArgument, "解析上传表单失败", err))
		return
	}

	file, header, err := r.FormFile("database")
	if err != nil {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "缺少数据库文件"))
		return
	}
	defer file.Close()

	if header.Size > 500<<20 {
		sendError(w, apperr.New(apperr.CodeResourceExhausted, "数据库文件大小超过 500MB 限制"))
		return
	}

	ext := filepath.Ext(header.Filename)
	if ext != ".db" && ext != ".sqlite" && ext != ".sqlite3" {
		sendError(w, apperr.New(apperr.CodeInvalidArgument, "只支持 .db, .sqlite, .sqlite3 格式的数据库文件"))
		return
	}

	tempFile, err := os.CreateTemp("", "import_*.db")
	if err != nil {
		sendError(w, apperr.Wrap(apperr.CodeInternal, "创建临时文件失败", err))
		return
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	if _, err := io.Copy(tempFile, file); err != nil {
		tempFile.Close()
		sendError(w, apperr.Wrap(apperr.CodeInternal, "写入临时文件失败", err))
	}
	tempFile.Close()

	if err := h.service.ImportDatabase(tempPath); err != nil {
		sendError(w, err)
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "数据库导入成功",
	})
}
