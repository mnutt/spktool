package domain

import "time"

type ProviderName string

const (
	ProviderVagrant ProviderName = "vagrant"
	ProviderLima    ProviderName = "lima"
)

type ProjectState struct {
	SchemaVersion int          `json:"schemaVersion"`
	Migration     int          `json:"migration"`
	Provider      ProviderName `json:"provider"`
	VMInstance    string       `json:"vmInstance"`
	Stack         string       `json:"stack"`
	ToolVersion   string       `json:"toolVersion"`
	UpdatedAt     time.Time    `json:"updatedAt"`
}

func (s *ProjectState) Normalize() {
	if s.SchemaVersion == 0 {
		s.SchemaVersion = 1
	}
	if s.UpdatedAt.IsZero() {
		s.UpdatedAt = time.Now().UTC()
	}
}
