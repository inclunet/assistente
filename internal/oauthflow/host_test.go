package oauthflow

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestHostIDHelperProcess(t *testing.T) {
	dir := os.Getenv("ASSISTENTE_TEST_OAUTH_HOST_DIR")
	if dir == "" {
		return
	}
	id, err := hostIDAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("ASSISTENTE_TEST_OAUTH_HOST_RESULT"), []byte(id), 0600); err != nil {
		t.Fatal(err)
	}
}
func TestHostIDAtomicAcrossProcesses(t *testing.T) {
	dir := t.TempDir()
	// An interrupted writer must not poison the canonical ID.
	if err := os.WriteFile(filepath.Join(dir, ".oauth-host-id-interrupted"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	const count = 8
	var wg sync.WaitGroup
	for n := range count {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			result := filepath.Join(dir, string(rune('a'+n)))
			cmd := exec.Command(executable, "-test.run=^TestHostIDHelperProcess$")
			cmd.Env = append(os.Environ(), "ASSISTENTE_TEST_OAUTH_HOST_DIR="+dir, "ASSISTENTE_TEST_OAUTH_HOST_RESULT="+result)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("child: %v %s", err, output)
			}
		}(n)
	}
	wg.Wait()
	id, err := hostIDAt(dir)
	if err != nil || !strings.HasPrefix(id, "urn:uuid:") {
		t.Fatalf("id: %q %v", id, err)
	}
	for n := range count {
		result, err := os.ReadFile(filepath.Join(dir, string(rune('a'+n))))
		if err != nil || string(result) != id {
			t.Fatalf("different process ID: %q %v", result, err)
		}
	}
}
