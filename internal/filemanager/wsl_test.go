package filemanager

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestWSLPathNormalization(t *testing.T) {
	wslPaths := []string{
		`\\wsl$\Ubuntu\home\user\file.go`,
		`\\wsl.localhost\Ubuntu-24.04\home\user\file.go`,
		`\\wsl$\Ubuntu-24.04\home\user\projeto\main.go`,
	}

	for _, path := range wslPaths {
		t.Run(path, func(t *testing.T) {
			// Testa como filepath.Abs processa o caminho
			absPath, err := filepath.Abs(path)
			t.Logf("Original: %s", path)
			t.Logf("Abs:      %s (err: %v)", absPath, err)

			// Verifica se isWSLPath funciona após Abs
			lowerPath := strings.ToLower(absPath)
			isWSL := strings.HasPrefix(lowerPath, `\\wsl$\`) ||
				strings.HasPrefix(lowerPath, `\\wsl.localhost\`) ||
				strings.HasPrefix(lowerPath, `//wsl$/`) ||
				strings.HasPrefix(lowerPath, `//wsl.localhost/`)
			t.Logf("isWSLPath após Abs: %v", isWSL)
			t.Logf("lowerPath: %s", lowerPath)
		})
	}
}

func TestValidateWSLPathForRead(t *testing.T) {
	sv := NewSecurityValidator(nil)

	path := `\\wsl.localhost\Ubuntu-24.04\home\user\projeto\main.go`
	
	t.Logf("Testing path: %s", path)
	
	// Simula o que ValidatePathForOperation faz
	absPath, err := filepath.Abs(path)
	t.Logf("After filepath.Abs: %s (err: %v)", absPath, err)
	
	// Testa isWSLPath diretamente
	lowerPath := strings.ToLower(absPath)
	t.Logf("Lower path: %s", lowerPath)
	t.Logf("Has prefix \\\\wsl$\\: %v", strings.HasPrefix(lowerPath, `\\wsl$\`))
	t.Logf("Has prefix \\\\wsl.localhost\\: %v", strings.HasPrefix(lowerPath, `\\wsl.localhost\`))
	
	// Testa a validação completa
	err = sv.ValidatePathForOperation(path, OpRead)
	t.Logf("ValidatePathForOperation result: %v", err)
	
	if err == ErrProtectedPath {
		t.Errorf("WSL path should NOT be blocked as protected path!")
	}
}



