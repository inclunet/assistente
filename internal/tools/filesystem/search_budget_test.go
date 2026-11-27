package filesystem

import (
	"context"
	"encoding/json"
	"testing"
	"testing/synctest"
	"time"

	"assistente/internal/tools"
)

func TestSearchBudgetStopsWithoutAutomaticRetry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		ctx, finish := startSearchBudget(t.Context())
		<-ctx.Done()
		var result tools.ToolResult
		err := ctx.Err()
		finish(&result, &err)
		if err != nil || result.Failure == nil || result.Failure.Kind != tools.ErrorKindTimeout || result.Failure.Retryable || time.Since(start) != 30*time.Second {
			t.Fatalf("tempo e classificação da busca: %+v, %v", result, err)
		}
	})
}

type slowSearchApproval struct{ stubPathAuthorizer }

func (slowSearchApproval) Authorize(ctx context.Context, _, _ string) error {
	select {
	case <-time.After(35 * time.Second):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func TestSearchApprovalDoesNotConsumeScanBudget(t *testing.T) {
	workDir, outside := t.TempDir(), t.TempDir()
	previous := pathAuthorizer
	SetPathAuthorizer(slowSearchApproval{})
	t.Cleanup(func() { SetPathAuthorizer(previous) })
	args, err := json.Marshal(map[string]string{"path": outside, "pattern": "sample"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range []tools.Tool{NewSearchFiles(workDir), NewGrepSearch(workDir)} {
		t.Run(tool.Name(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				start := time.Now()
				result, err := tool.Execute(t.Context(), args)
				if err != nil || result.IsError || time.Since(start) < 35*time.Second {
					t.Fatalf("decisão e busca: %+v %v", result, err)
				}
			})
		})
	}
}
