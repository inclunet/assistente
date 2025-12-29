package filemanager

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlainTextHandler_ReadGoFile(t *testing.T) {
	h := NewPlainTextHandler()
	tmpDir := t.TempDir()

	// Cria um arquivo .go
	testFile := filepath.Join(tmpDir, "main.go")
	goCode := `package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
}
`
	os.WriteFile(testFile, []byte(goCode), 0644)

	content, err := h.ReadContent(testFile, ReadOptions{})
	if err != nil {
		t.Fatalf("ReadContent failed for .go file: %v", err)
	}

	if content.Text != goCode {
		t.Errorf("Content mismatch.\nGot: %q\nWant: %q", content.Text, goCode)
	}

	t.Logf("Successfully read .go file with %d lines", content.LineCount)
}

func TestPlainTextHandler_ReadJSFile(t *testing.T) {
	h := NewPlainTextHandler()
	tmpDir := t.TempDir()

	testFile := filepath.Join(tmpDir, "app.js")
	jsCode := `const express = require('express');
const app = express();

app.get('/', (req, res) => {
  res.send('Hello World!');
});

app.listen(3000);
`
	os.WriteFile(testFile, []byte(jsCode), 0644)

	content, err := h.ReadContent(testFile, ReadOptions{})
	if err != nil {
		t.Fatalf("ReadContent failed for .js file: %v", err)
	}

	if content.Text != jsCode {
		t.Errorf("Content mismatch for .js file")
	}

	t.Logf("Successfully read .js file with %d lines", content.LineCount)
}

func TestPlainTextHandler_ReadCSSFile(t *testing.T) {
	h := NewPlainTextHandler()
	tmpDir := t.TempDir()

	testFile := filepath.Join(tmpDir, "styles.css")
	cssCode := `body {
  font-family: Arial, sans-serif;
  background-color: #f0f0f0;
}

.container {
  max-width: 1200px;
  margin: 0 auto;
}
`
	os.WriteFile(testFile, []byte(cssCode), 0644)

	content, err := h.ReadContent(testFile, ReadOptions{})
	if err != nil {
		t.Fatalf("ReadContent failed for .css file: %v", err)
	}

	if content.Text != cssCode {
		t.Errorf("Content mismatch for .css file")
	}

	t.Logf("Successfully read .css file with %d lines", content.LineCount)
}

func TestPlainTextHandler_ReadTSFile(t *testing.T) {
	h := NewPlainTextHandler()
	tmpDir := t.TempDir()

	testFile := filepath.Join(tmpDir, "app.ts")
	tsCode := `interface User {
  name: string;
  age: number;
}

function greet(user: User): string {
  return "Hello, " + user.name;
}
`
	os.WriteFile(testFile, []byte(tsCode), 0644)

	content, err := h.ReadContent(testFile, ReadOptions{})
	if err != nil {
		t.Fatalf("ReadContent failed for .ts file: %v", err)
	}

	if content.Text != tsCode {
		t.Errorf("Content mismatch for .ts file")
	}

	t.Logf("Successfully read .ts file with %d lines", content.LineCount)
}

func TestPlainTextHandler_ReadPythonFile(t *testing.T) {
	h := NewPlainTextHandler()
	tmpDir := t.TempDir()

	testFile := filepath.Join(tmpDir, "app.py")
	pyCode := `def hello(name: str) -> str:
    return f"Hello, {name}!"

if __name__ == "__main__":
    print(hello("World"))
`
	os.WriteFile(testFile, []byte(pyCode), 0644)

	content, err := h.ReadContent(testFile, ReadOptions{})
	if err != nil {
		t.Fatalf("ReadContent failed for .py file: %v", err)
	}

	if content.Text != pyCode {
		t.Errorf("Content mismatch for .py file")
	}

	t.Logf("Successfully read .py file with %d lines", content.LineCount)
}

func TestLocalStorageProvider_ReadCodeFiles(t *testing.T) {
	p := NewLocalStorageProvider()
	ctx := context.Background()
	tmpDir := t.TempDir()

	// Cria vários arquivos de código
	files := map[string]string{
		"main.go":      "package main\n\nfunc main() {}\n",
		"app.js":       "console.log('Hello');\n",
		"styles.css":   "body { color: red; }\n",
		"index.ts":     "const x: number = 1;\n",
		"script.py":    "print('hello')\n",
		"Component.svelte": "<script>\n  let count = 0;\n</script>\n",
	}

	for filename, content := range files {
		filePath := filepath.Join(tmpDir, filename)
		os.WriteFile(filePath, []byte(content), 0644)

		t.Run(filename, func(t *testing.T) {
			result, err := p.ReadFile(ctx, filePath, ReadOptions{})
			if err != nil {
				t.Errorf("Failed to read %s: %v", filename, err)
				return
			}

			if result.Text != content {
				t.Errorf("Content mismatch for %s.\nGot: %q\nWant: %q", filename, result.Text, content)
			}

			t.Logf("✅ %s read successfully (%d bytes)", filename, len(result.Text))
		})
	}
}

func TestStorageManager_ReadCodeFiles(t *testing.T) {
	sm := NewStorageManager()
	ctx := context.Background()
	tmpDir := t.TempDir()

	// Cria arquivo Go
	goFile := filepath.Join(tmpDir, "test.go")
	goCode := "package main\n"
	os.WriteFile(goFile, []byte(goCode), 0644)

	content, err := sm.ReadFile(ctx, goFile, ReadOptions{})
	if err != nil {
		t.Fatalf("StorageManager.ReadFile failed for .go: %v", err)
	}

	if !strings.Contains(content.Text, "package main") {
		t.Error("Expected to find 'package main' in content")
	}

	t.Logf("✅ StorageManager successfully read .go file")
}

