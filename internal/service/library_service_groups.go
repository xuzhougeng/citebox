package service

import (
	"strings"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

func (s *LibraryService) ListGroups() ([]model.Group, error) {
	return s.repo.ListGroups()
}

func (s *LibraryService) CreateGroup(name, description string) (*model.Group, error) {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if name == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "分组名称不能为空")
	}
	return s.repo.CreateGroup(name, description)
}

func (s *LibraryService) GetOrCreateGroupByName(name, description string) (*model.Group, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "分组名称不能为空")
	}
	existing, err := s.repo.GetGroupByName(name)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}
	group, err := s.CreateGroup(name, description)
	if err == nil {
		return group, nil
	}
	if !apperr.IsCode(err, apperr.CodeConflict) {
		return nil, err
	}
	return s.repo.GetGroupByName(name)
}

func (s *LibraryService) UpdateGroup(id int64, name, description string) (*model.Group, error) {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if name == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "分组名称不能为空")
	}
	return s.repo.UpdateGroup(id, name, description)
}

func (s *LibraryService) DeleteGroup(id int64) error {
	return s.repo.DeleteGroup(id)
}

func (s *LibraryService) ListTags(scope model.TagScope) ([]model.Tag, error) {
	return s.repo.ListTags(scope)
}

func (s *LibraryService) CreateTag(scope model.TagScope, name, color string) (*model.Tag, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "标签名称不能为空")
	}
	color = normalizeColor(color)
	return s.repo.CreateTag(model.NormalizeTagScope(string(scope)), name, color)
}

func (s *LibraryService) UpdateTag(id int64, name, color string) (*model.Tag, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "标签名称不能为空")
	}
	return s.repo.UpdateTag(id, name, normalizeColor(color))
}

func (s *LibraryService) DeleteTag(id int64) error {
	return s.repo.DeleteTag(id)
}
