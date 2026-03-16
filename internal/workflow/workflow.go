package workflow

import (
	"context"
	"fmt"

	"github.com/mnutt/spktool/internal/domain"
)

type Step struct {
	Name     string
	Do       func(context.Context) error
	Rollback func(context.Context) error
}

type ExecutionError struct {
	Workflow       string  `json:"workflow"`
	Step           string  `json:"step"`
	Err            error   `json:"-"`
	RollbackErrors []error `json:"-"`
}

func (e *ExecutionError) Error() string {
	if len(e.RollbackErrors) > 0 {
		return fmt.Sprintf("workflow %s failed at step %s: %v (%d rollback errors)", e.Workflow, e.Step, e.Err, len(e.RollbackErrors))
	}
	return fmt.Sprintf("workflow %s failed at step %s: %v", e.Workflow, e.Step, e.Err)
}

func (e *ExecutionError) Unwrap() error { return e.Err }

func Run(ctx context.Context, name string, steps []Step) error {
	completed := make([]Step, 0, len(steps))
	for _, step := range steps {
		if err := step.Do(ctx); err != nil {
			execErr := &ExecutionError{Workflow: name, Step: step.Name, Err: err}
			for i := len(completed) - 1; i >= 0; i-- {
				if completed[i].Rollback == nil {
					continue
				}
				if rollbackErr := completed[i].Rollback(ctx); rollbackErr != nil {
					execErr.RollbackErrors = append(execErr.RollbackErrors, rollbackErr)
				}
			}
			return &domain.Error{
				Code:    domain.ErrWorkflow,
				Op:      "workflow.Run",
				Message: execErr.Error(),
				Cause:   execErr,
			}
		}
		completed = append(completed, step)
	}
	return nil
}
