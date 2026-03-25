package service

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
)

func (s *LibraryService) DatabasePath() string {
	return s.config.DatabasePath
}

func (s *LibraryService) GetRuntimePassword() string {
	password, err := s.repo.GetAppSetting(runtimePasswordKey)
	if err != nil {
		s.logger.Warn("failed to get runtime password", "error", err)
	}
	if strings.TrimSpace(password) != "" {
		return password
	}
	return s.config.AdminPassword
}

func (s *LibraryService) ChangePassword(currentPassword, newPassword string) error {
	currentPassword = strings.TrimSpace(currentPassword)
	newPassword = strings.TrimSpace(newPassword)

	if newPassword == "" {
		return apperr.New(apperr.CodeInvalidArgument, "新密码不能为空")
	}
	if len(newPassword) < 6 {
		return apperr.New(apperr.CodeInvalidArgument, "新密码长度不能少于 6 位")
	}

	runtimePassword := s.GetRuntimePassword()
	if currentPassword != runtimePassword {
		return apperr.New(apperr.CodeUnauthenticated, "当前密码错误")
	}

	if err := s.repo.UpsertAppSetting(runtimePasswordKey, newPassword); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "保存新密码失败", err)
	}

	s.logger.Info("admin password changed successfully")
	return nil
}

func (s *LibraryService) ValidateCredentials(username, password string) bool {
	expectedUsername := s.config.AdminUsername
	expectedPassword := s.GetRuntimePassword()
	return username == expectedUsername && password == expectedPassword
}

func (s *LibraryService) GetAuthSettings() model.AuthSettings {
	return model.AuthSettings{
		Username:       s.config.AdminUsername,
		PasswordFromDB: s.GetRuntimePassword() != s.config.AdminPassword,
		WeixinBinding:  s.getWeixinBindingSummary(),
		WeixinBridge:   s.getWeixinBridgeSettingsSummary(),
	}
}

func (s *LibraryService) ImportDatabase(sourcePath string) error {
	s.repoMu.Lock()
	defer s.repoMu.Unlock()

	if err := s.repo.Close(); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "关闭当前数据库失败", err)
	}

	dbPath := s.config.DatabasePath
	backupPath := dbPath + ".backup." + time.Now().Format("20060102150405")

	if err := copyFile(dbPath, backupPath); err != nil {
		_ = s.reopenRepo()
		return apperr.Wrap(apperr.CodeInternal, "备份当前数据库失败", err)
	}

	if err := copyFile(sourcePath, dbPath); err != nil {
		_ = copyFile(backupPath, dbPath)
		_ = s.reopenRepo()
		return apperr.Wrap(apperr.CodeInternal, "替换数据库文件失败", err)
	}

	if err := s.reopenRepo(); err != nil {
		_ = copyFile(backupPath, dbPath)
		_ = s.reopenRepo()
		return apperr.Wrap(apperr.CodeInternal, "重新打开数据库失败", err)
	}

	return nil
}

func (s *LibraryService) reopenRepo() error {
	repo, err := repository.NewLibraryRepository(s.config.DatabasePath)
	if err != nil {
		return err
	}
	s.repo = repo
	return nil
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return err
	}

	return destFile.Sync()
}
