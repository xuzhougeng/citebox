package agent_session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/xuzhougeng/citebox/internal/service/agent_session/commands"
)

type surfaceStateData struct {
	Surfaces map[string]commands.SurfaceContext `json:"surfaces"`
}

type SurfaceStateStore struct {
	path string
	mu   sync.Mutex
	data surfaceStateData
}

func NewSurfaceStateStore(path string) (*SurfaceStateStore, error) {
	s := &SurfaceStateStore{
		path: path,
		data: surfaceStateData{Surfaces: map[string]commands.SurfaceContext{}},
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *SurfaceStateStore) load() error {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(raw, &s.data)
}

func (s *SurfaceStateStore) save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *SurfaceStateStore) Get(surface string) (commands.SurfaceContext, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.Surfaces[surface], nil
}

func (s *SurfaceStateStore) Reset(surface string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data.Surfaces, surface)
	return s.save()
}

func (s *SurfaceStateStore) SetCurrentPaper(surface string, paperID int64, title string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sc := s.data.Surfaces[surface]
	sc.CurrentPaperID = paperID
	sc.CurrentPaperTitle = title
	s.data.Surfaces[surface] = sc
	return s.save()
}

func (s *SurfaceStateStore) SetCurrentFigure(surface string, figureID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sc := s.data.Surfaces[surface]
	sc.CurrentFigureID = figureID
	s.data.Surfaces[surface] = sc
	return s.save()
}

func (s *SurfaceStateStore) SetSearchResults(surface string, paperIDs, figureIDs []int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sc := s.data.Surfaces[surface]
	sc.RecentSearchPaperIDs = paperIDs
	sc.RecentSearchFigureIDs = figureIDs
	s.data.Surfaces[surface] = sc
	return s.save()
}
