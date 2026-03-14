package workflow_test

import (
	"context"
	"errors"
	"testing"

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
