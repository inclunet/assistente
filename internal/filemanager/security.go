package filemanager

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Pastas que NÃO PODEM ser acessadas de nenhuma forma (exceto list/info básico)
var protectedPaths = []string{}

// Extensões que NÃO PODEM ser lidas, escritas ou deletadas
var protectedExtensions = []string{}

// Arquivos específicos protegidos (qualquer lugar)
var protectedFiles = []string{}

func init() {
	if runtime.GOOS == "windows" {
		protectedPaths = []string{
			// Sistema Windows
			"C:\\Windows",
			"C:\\Windows\\System32",
			"C:\\Windows\\SysWOW64",
			"C:\\Program Files",
			"C:\\Program Files (x86)",
			"C:\\ProgramData",
			"C:\\Recovery",
			"C:\\$Recycle.Bin",

			// Dados sensíveis do usuário (relativos ao USERPROFILE)
			"AppData\\Local\\Microsoft",
			"AppData\\Roaming\\Microsoft",
			"AppData\\Local\\Google\\Chrome\\User Data",
			"AppData\\Local\\Microsoft\\Edge\\User Data",
			"AppData\\Roaming\\Mozilla\\Firefox\\Profiles",

			// Credenciais e chaves
			".ssh",
			".gnupg",
			".aws",
			".azure",
			".kube",
			".docker",
		}

		protectedExtensions = []string{
			// Executáveis e bibliotecas
			".dll", ".sys", ".exe", ".com", ".scr",
			".drv", ".ocx", ".cpl", ".msc",

			// Scripts de sistema
			".bat", ".cmd", ".ps1", ".psm1", ".psd1",
			".vbs", ".vbe", ".wsf", ".wsh",

			// Instaladores e registro
			".msi", ".msp", ".msu", ".reg", ".inf",

			// Arquivos de boot
			".efi",
		}

		protectedFiles = []string{
			"boot.ini",
			"ntldr",
			"bootmgr",
			"pagefile.sys",
			"hiberfil.sys",
			"swapfile.sys",
			"desktop.ini",
			"thumbs.db",
			"NTUSER.DAT",
		}
	} else {
		// Linux/macOS
		protectedPaths = []string{
			"/bin",
			"/sbin",
			"/usr/bin",
			"/usr/sbin",
			"/usr/lib",
			"/lib",
			"/lib64",
			"/boot",
			"/etc",
			"/root",
			"/var/log",

			// Credenciais
			".ssh",
			".gnupg",
			".aws",
			".azure",
			".kube",
			".docker",
		}

		protectedExtensions = []string{
			".so",
			".dylib",
		}

		protectedFiles = []string{
			"passwd",
			"shadow",
			"sudoers",
		}
	}
}

// SecurityValidator valida operações de arquivo
type SecurityValidator struct {
	authorizedPaths []AuthorizedPath
}

// NewSecurityValidator cria um novo validador de segurança
func NewSecurityValidator(authorizedPaths []AuthorizedPath) *SecurityValidator {
	return &SecurityValidator{
		authorizedPaths: authorizedPaths,
	}
}

// SetAuthorizedPaths atualiza as pastas autorizadas
func (v *SecurityValidator) SetAuthorizedPaths(paths []AuthorizedPath) {
	v.authorizedPaths = paths
}

// ValidatePathForOperation verifica se um caminho é seguro para a operação
func (v *SecurityValidator) ValidatePathForOperation(path string, op Operation) error {
	// 1. Normalizar caminho (resolver .., ., links simbólicos)
	absPath, err := filepath.Abs(path)
	if err != nil {
		return ErrPathTraversal
	}

	// Tenta resolver links simbólicos para evitar bypass
	realPath, err := filepath.EvalSymlinks(absPath)
	if err == nil {
		absPath = realPath
	}

	// 2. Verifica se está em pasta protegida
	// BLOQUEIA: read, write, delete (permite: list, info básico)
	if isProtectedPath(absPath) {
		if op != OpList && op != OpInfo {
			return ErrProtectedPath
		}
	}

	// 3. Verifica extensão protegida
	// BLOQUEIA TODAS as operações para extensões de sistema
	ext := strings.ToLower(filepath.Ext(absPath))
	if isProtectedExtension(ext) {
		return ErrProtectedExtension
	}

	// 4. Verifica se é arquivo específico protegido
	fileName := strings.ToLower(filepath.Base(absPath))
	if isProtectedFile(fileName) {
		return ErrProtectedFile
	}

	// 5. Para operações de delete, verifica autorização
	//    ~/.assistente/ é sempre autorizado para delete (dados do app: memórias, skills, configs)
	if op == OpDelete {
		if !isAppDataPath(absPath) && !v.isPathAuthorizedForDelete(absPath) {
			return ErrDeleteNotAllowed
		}
	}

	return nil
}

// isAppDataPath verifica se o caminho está dentro de ~/.assistente/ (dados do app)
func isAppDataPath(absPath string) bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	appDir := strings.ToLower(filepath.Join(home, ".assistente"))
	lowerPath := strings.ToLower(absPath)
	return strings.HasPrefix(lowerPath, appDir+string(os.PathSeparator)) || lowerPath == appDir
}

// isProtectedPath verifica se o caminho está em uma pasta protegida
func isProtectedPath(absPath string) bool {
	lowerPath := strings.ToLower(absPath)

	// Caminhos WSL não são considerados pastas de sistema do Windows
	// Eles são sistemas de arquivos separados (Linux) acessíveis via rede
	if isWSLPath(lowerPath) {
		return false
	}

	for _, protected := range protectedPaths {
		protectedLower := strings.ToLower(protected)

		// Se é caminho relativo (como .ssh), verifica se está contido
		if !filepath.IsAbs(protected) {
			if strings.Contains(lowerPath, string(os.PathSeparator)+protectedLower+string(os.PathSeparator)) ||
				strings.HasSuffix(lowerPath, string(os.PathSeparator)+protectedLower) {
				return true
			}
			continue
		}

		// Verifica se é a pasta exata ou está dentro dela
		if strings.HasPrefix(lowerPath, protectedLower) {
			// Garante que é a pasta exata ou subpasta (não apenas prefixo de nome)
			if len(lowerPath) == len(protectedLower) ||
				lowerPath[len(protectedLower)] == os.PathSeparator {
				return true
			}
		}
	}
	return false
}

// isWSLPath verifica se o caminho é um path do WSL (Windows Subsystem for Linux)
func isWSLPath(path string) bool {
	lowerPath := strings.ToLower(path)
	// Formatos de caminho WSL:
	// \\wsl$\Ubuntu\...
	// \\wsl.localhost\Ubuntu\...
	// \\wsl$\Ubuntu-24.04\...
	return strings.HasPrefix(lowerPath, `\\wsl$\`) ||
		strings.HasPrefix(lowerPath, `\\wsl.localhost\`) ||
		strings.HasPrefix(lowerPath, `//wsl$/`) ||
		strings.HasPrefix(lowerPath, `//wsl.localhost/`)
}

// isProtectedExtension verifica se a extensão é protegida
func isProtectedExtension(ext string) bool {
	ext = strings.ToLower(ext)
	for _, protected := range protectedExtensions {
		if ext == protected {
			return true
		}
	}
	return false
}

// isProtectedFile verifica se é um arquivo específico protegido
func isProtectedFile(fileName string) bool {
	fileName = strings.ToLower(fileName)
	for _, protected := range protectedFiles {
		if fileName == strings.ToLower(protected) {
			return true
		}
	}
	return false
}

// isPathAuthorizedForDelete verifica se o caminho está em uma pasta autorizada para delete
func (v *SecurityValidator) isPathAuthorizedForDelete(absPath string) bool {
	lowerPath := strings.ToLower(absPath)

	for _, auth := range v.authorizedPaths {
		if !auth.AllowDelete {
			continue
		}

		authPath := strings.ToLower(auth.Path)
		if !filepath.IsAbs(authPath) {
			var err error
			authPath, err = filepath.Abs(authPath)
			if err != nil {
				continue
			}
			authPath = strings.ToLower(authPath)
		}

		// Verifica se está dentro da pasta autorizada
		if strings.HasPrefix(lowerPath, authPath) {
			remaining := lowerPath[len(authPath):]

			// É exatamente a pasta ou arquivo dentro dela
			if remaining == "" || remaining[0] == os.PathSeparator {
				// Se não é recursivo, só permite arquivos diretos (não subpastas)
				if !auth.Recursive && strings.Count(remaining, string(os.PathSeparator)) > 1 {
					continue
				}
				return true
			}
		}
	}

	return false
}

// IsPathAuthorizedForWrite verifica se o caminho está em uma pasta autorizada para escrita
func (v *SecurityValidator) IsPathAuthorizedForWrite(absPath string) bool {
	lowerPath := strings.ToLower(absPath)

	for _, auth := range v.authorizedPaths {
		if !auth.AllowWrite {
			continue
		}

		authPath := strings.ToLower(auth.Path)
		if !filepath.IsAbs(authPath) {
			var err error
			authPath, err = filepath.Abs(authPath)
			if err != nil {
				continue
			}
			authPath = strings.ToLower(authPath)
		}

		if strings.HasPrefix(lowerPath, authPath) {
			remaining := lowerPath[len(authPath):]
			if remaining == "" || remaining[0] == os.PathSeparator {
				if !auth.Recursive && strings.Count(remaining, string(os.PathSeparator)) > 1 {
					continue
				}
				return true
			}
		}
	}

	return false
}

// CanAccessPath verifica se pode acessar um caminho (qualquer operação)
func (v *SecurityValidator) CanAccessPath(path string) bool {
	err := v.ValidatePathForOperation(path, OpRead)
	return err == nil
}

// GetProtectedPaths retorna a lista de pastas protegidas (para UI)
func GetProtectedPaths() []string {
	return protectedPaths
}

// GetProtectedExtensions retorna a lista de extensões protegidas (para UI)
func GetProtectedExtensions() []string {
	return protectedExtensions
}

// GetProtectedFiles retorna a lista de arquivos protegidos (para UI)
func GetProtectedFiles() []string {
	return protectedFiles
}

