package state_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/mnutt/spktool/internal/domain"
	"github.com/mnutt/spktool/internal/state"
)

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := state.New()
	input := &domain.ProjectState{
		Provider:    domain.ProviderLima,
		VMInstance:  "sandstorm-demo",
		Stack:       "lemp",
		ToolVersion: "0.1.0",
		Migration:   1,
	}
	if err := store.Save(context.Background(), dir, input); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Provider != input.Provider || loaded.Stack != input.Stack {
		t.Fatalf("loaded = %+v", loaded)
	}
	if got := store.Path(dir); got != filepath.Join(dir, ".sandstorm", state.FileName) {
		t.Fatalf("unexpected path: %s", got)
	}
}
