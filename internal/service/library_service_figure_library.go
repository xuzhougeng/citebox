package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

const figureLibrarySettingsKey = "figure_library_settings"

func defaultFigureLibrarySettings() model.FigureLibrarySettings {
	return model.FigureLibrarySettings{}
}

func (s *LibraryService) GetFigureLibrarySettings() (*model.FigureLibrarySettings, error) {
	raw, err := s.repo.GetAppSetting(figureLibrarySettingsKey)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "读取 Figure Library 配置失败", err)
	}
	settings := defaultFigureLibrarySettings()
	if strings.TrimSpace(raw) == "" {
		return &settings, nil
	}
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "解析 Figure Library 配置失败", err)
	}
	normalized, err := normalizeFigureLibrarySettings(settings, false)
	if err != nil {
		return nil, err
	}
	return &normalized, nil
}

func (s *LibraryService) UpdateFigureLibrarySettings(input model.FigureLibrarySettings) (*model.FigureLibrarySettings, error) {
	settings, err := normalizeFigureLibrarySettings(input, true)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(settings)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "序列化 Figure Library 配置失败", err)
	}
	if err := s.repo.UpsertAppSetting(figureLibrarySettingsKey, string(payload)); err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "保存 Figure Library 配置失败", err)
	}
	return &settings, nil
}

func (s *LibraryService) GetFigureLibraryStatus() (*model.FigureLibraryStatus, error) {
	settings, err := s.GetFigureLibrarySettings()
	if err != nil {
		return nil, err
	}
	status := &model.FigureLibraryStatus{
		DropDir: settings.DropDir,
	}
	if strings.TrimSpace(settings.DropDir) == "" {
		status.Message = "还没有设置接收目录"
		return status, nil
	}
	status.Configured = true
	if err := ensureFigureLibraryDropDir(settings.DropDir); err != nil {
		status.Message = err.Error()
		return status, nil
	}
	status.Ready = true
	status.Message = "接收目录可用"
	return status, nil
}

func (s *LibraryService) SendFigureToFigureLibrary(id int64) (*model.FigureLibrarySendResult, error) {
	settings, err := s.GetFigureLibrarySettings()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(settings.DropDir) == "" {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "请先在设置中填写 Figure Library 接收目录")
	}
	if err := ensureFigureLibraryDropDir(settings.DropDir); err != nil {
		return nil, apperr.New(apperr.CodeFailedPrecondition, err.Error())
	}
	pkg, err := s.ExportFigureTransferPackage(id)
	if err != nil {
		return nil, err
	}
	target := filepath.Join(settings.DropDir, pkg.Filename)
	if err := os.WriteFile(target, pkg.Data, 0o644); err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "写入 Figure Library 接收目录失败", err)
	}
	return &model.FigureLibrarySendResult{
		FigureID: id,
		Filename: pkg.Filename,
		Path:     target,
	}, nil
}

func normalizeFigureLibrarySettings(input model.FigureLibrarySettings, requireExisting bool) (model.FigureLibrarySettings, error) {
	dir := strings.TrimSpace(input.DropDir)
	if dir == "" {
		return model.FigureLibrarySettings{}, nil
	}
	cleaned := filepath.Clean(dir)
	if !filepath.IsAbs(cleaned) {
		return model.FigureLibrarySettings{}, apperr.New(apperr.CodeInvalidArgument, "接收目录必须是本机绝对路径")
	}
	if requireExisting {
		if err := ensureFigureLibraryDropDir(cleaned); err != nil {
			return model.FigureLibrarySettings{}, apperr.New(apperr.CodeInvalidArgument, err.Error())
		}
	}
	return model.FigureLibrarySettings{DropDir: cleaned}, nil
}

func ensureFigureLibraryDropDir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return apperr.New(apperr.CodeInvalidArgument, "接收目录不存在")
		}
		return err
	}
	if !info.IsDir() {
		return apperr.New(apperr.CodeInvalidArgument, "接收路径不是目录")
	}
	probe := filepath.Join(dir, ".citebox-figure-library-write-test")
	if err := os.WriteFile(probe, []byte("ok"), 0o644); err != nil {
		return apperr.New(apperr.CodeInvalidArgument, "接收目录不可写")
	}
	_ = os.Remove(probe)
	return nil
}
