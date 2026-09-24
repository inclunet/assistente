package app

import (
	"runtime"
	"testing"

	"assistente/internal/commandexecution"
)

func TestExternalCommandProviderUsesProductObservedAfterLockWait(t *testing.T) {
	a := &App{}
	oldProduct := &commandProductRuntime{}
	currentProduct := &commandProductRuntime{}
	a.commandProduct.Store(oldProduct)
	provider := &appExternalCommandProvider{app: a}
	provider.mu.Lock()
	done := make(chan *commandexecution.ExternalService, 1)
	started := make(chan struct{})
	go func() {
		close(started)
		done <- provider.ExternalCommandService("ui")
	}()
	<-started
	// Give the caller a scheduling opportunity while it is blocked on the
	// provider mutex, then rotate the product before allowing publication.
	for i := 0; i < 64; i++ {
		runtime.Gosched()
	}
	a.commandProduct.Store(currentProduct)
	provider.mu.Unlock()
	if service := <-done; service != nil {
		t.Fatal("provider without auth/bootstrap dependencies unexpectedly built an executor")
	}
	if provider.product != currentProduct {
		t.Fatal("provider published the product captured before the lock wait")
	}
}
