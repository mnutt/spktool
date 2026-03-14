package state

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/mnutt/spktool/internal/domain"
)

const FileName = "project-state.json"

type Store struct{}

func New() *Store { return &Store{} }

func (s *Store) Path(workDir string) string {
	return filepath.Join(workDir, ".sandstorm", FileName)
}

func (s *Store) Load(_ context.Context, workDir string) (*domain.ProjectState, error) {
	path := s.Path(workDir)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, domain.Wrap(domain.ErrExternal, "state.Load", "read project state", err)
	}

	var st domain.ProjectState
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, domain.Wrap(domain.ErrExternal, "state.Load", "decode project state", err)
	}
	st.Normalize()
	return &st, nil
}

func (s *Store) Save(_ context.Context, workDir string, st *domain.ProjectState) error {
	if err := os.MkdirAll(filepath.Join(workDir, ".sandstorm"), 0o755); err != nil {
		return domain.Wrap(domain.ErrExternal, "state.Save", "create .sandstorm directory", err)
	}

	st.UpdatedAt = time.Now().UTC()
	st.Normalize()
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return domain.Wrap(domain.ErrExternal, "state.Save", "encode project state", err)
	}

	path := s.Path(workDir)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return domain.Wrap(domain.ErrExternal, "state.Save", "write temp state file", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return domain.Wrap(domain.ErrExternal, "state.Save", "commit state file", err)
	}
	return nil
}
