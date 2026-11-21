package terminal

import (
	"errors"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// Um handle pode manter uma entrada pendente de exclusão no Windows mesmo
// após a saída do shell. testing.TempDir não repete ERROR_DIR_NOT_EMPTY.
func ptyTestDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "pty-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		err := removePTYTestDir(func() error { return os.RemoveAll(dir) }, 2*time.Second)
		if err != nil {
			entries, readErr := os.ReadDir(dir)
			t.Errorf("limpeza de %q: %v; entradas restantes: %v (leitura: %v)", dir, err, entries, readErr)
		}
	})
	return dir
}

func removePTYTestDir(remove func() error, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		err := remove()
		if !errors.Is(err, windows.ERROR_DIR_NOT_EMPTY) || !time.Now().Before(deadline) {
			return err
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func TestPTYDirectoryCleanupRetriesOnlyDirectoryNotEmpty(t *testing.T) {
	pending := &os.PathError{Op: "unlinkat", Path: "pty", Err: windows.ERROR_DIR_NOT_EMPTY}
	for _, tc := range []struct {
		name              string
		first, last, want error
		timeout           time.Duration
		calls             int
	}{
		{"transitório", pending, nil, nil, time.Second, 2},
		{"persistente", pending, pending, pending, 0, 1},
		{"outra falha", windows.ERROR_ACCESS_DENIED, nil, windows.ERROR_ACCESS_DENIED, time.Second, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			err := removePTYTestDir(func() error {
				calls++
				if calls == 1 {
					return tc.first
				}
				return tc.last
			}, tc.timeout)
			if !errors.Is(err, tc.want) || calls != tc.calls {
				t.Fatalf("erro %v, chamadas %d; esperado %v, %d", err, calls, tc.want, tc.calls)
			}
		})
	}
}
