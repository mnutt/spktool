package domain

type ProviderName string

const (
	ProviderVagrant ProviderName = "vagrant"
	ProviderLima    ProviderName = "lima"
)

type ProjectState struct {
	Provider    ProviderName `json:"provider"`
	VMInstance  string       `json:"vmInstance"`
	Stack       string       `json:"stack"`
	ToolVersion string       `json:"toolVersion"`
}
