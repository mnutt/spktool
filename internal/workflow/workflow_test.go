package workflow_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mnutt/spktool/internal/domain"
	"github.com/mnutt/spktool/internal/workflow"
)

func TestRunRollsBackCompletedSteps(t *testing.T) {
	var order []string
	err := workflow.Run(context.Background(), "test", []workflow.Step{
		{
			Name: "one",
			Do: func(context.Context) error {
				order = append(order, "do-one")
				return nil
			},
			Rollback: func(context.Context) error {
				order = append(order, "rollback-one")
				return nil
			},
		},
		{
			Name: "two",
			Do: func(context.Context) error {
				order = append(order, "do-two")
				return errors.New("boom")
			},
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	want := []string{"do-one", "do-two", "rollback-one"}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order[%d] = %q want %q", i, order[i], want[i])
		}
	}
}

func TestRunIncludesRollbackFailuresInError(t *testing.T) {
	err := workflow.Run(context.Background(), "test", []workflow.Step{
		{
			Name: "one",
			Do: func(context.Context) error {
				return nil
			},
			Rollback: func(context.Context) error {
				return errors.New("rollback failed")
			},
		},
		{
			Name: "two",
			Do: func(context.Context) error {
				return errors.New("boom")
			},
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) {
		t.Fatalf("expected domain error, got %T", err)
	}
	var execErr *workflow.ExecutionError
	if !errors.As(err, &execErr) {
		t.Fatalf("expected execution error, got %T", err)
	}
	if len(execErr.RollbackErrors) != 1 {
		t.Fatalf("expected rollback errors, got %+v", execErr.RollbackErrors)
	}
	if !strings.Contains(domainErr.Message, "1 rollback errors") {
		t.Fatalf("expected rollback failure count in message, got %q", domainErr.Message)
	}
}
