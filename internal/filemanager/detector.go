package filemanager

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Mapeamento de extensão para categoria
var extensionCategories = map[string]FileCategory{
	// Texto
	".txt": CategoryText, ".md": CategoryText, ".markdown": CategoryText,
	".log": CategoryText, ".csv": CategoryText, ".tsv": CategoryText,

	// Código
	".go": CategoryCode, ".py": CategoryCode, ".js": CategoryCode,
	".ts": CategoryCode, ".jsx": CategoryCode, ".tsx": CategoryCode,
	".java": CategoryCode, ".c": CategoryCode, ".cpp": CategoryCode,
	".h": CategoryCode, ".hpp": CategoryCode, ".cs": CategoryCode,
	".rb": CategoryCode, ".php": CategoryCode, ".rs": CategoryCode,
	".swift": CategoryCode, ".kt": CategoryCode, ".scala": CategoryCode,
	".sh": CategoryCode, ".bash": CategoryCode, ".zsh": CategoryCode,
	".ps1": CategoryCode, ".sql": CategoryCode, ".r": CategoryCode,
	".lua": CategoryCode, ".perl": CategoryCode, ".pl": CategoryCode,

	// Web
	".html": CategoryWeb, ".htm": CategoryWeb, ".css": CategoryWeb,
	".scss": CategoryWeb, ".less": CategoryWeb, ".sass": CategoryWeb,
	".vue": CategoryWeb, ".svelte": CategoryWeb,

	// Config
	".json": CategoryConfig, ".xml": CategoryConfig, ".yaml": CategoryConfig,
	".yml": CategoryConfig, ".ini": CategoryConfig, ".cfg": CategoryConfig,
	".conf": CategoryConfig, ".config": CategoryConfig, ".toml": CategoryConfig,
	".env": CategoryConfig, ".properties": CategoryConfig,

	// Documento
	".pdf": CategoryDocument, ".doc": CategoryDocument, ".docx": CategoryDocument,
	".xls": CategoryDocument, ".xlsx": CategoryDocument, ".ppt": CategoryDocument,
	".pptx": CategoryDocument, ".odt": CategoryDocument, ".ods": CategoryDocument,
	".odp": CategoryDocument, ".rtf": CategoryDocument, ".tex": CategoryDocument,
	".epub": CategoryDocument, ".mobi": CategoryDocument,

	// Imagem
	".jpg": CategoryImage, ".jpeg": CategoryImage, ".png": CategoryImage,
	".gif": CategoryImage, ".bmp": CategoryImage, ".webp": CategoryImage,
	".svg": CategoryImage, ".ico": CategoryImage, ".tiff": CategoryImage,
	".tif": CategoryImage, ".psd": CategoryImage, ".ai": CategoryImage,
	".raw": CategoryImage, ".heic": CategoryImage, ".heif": CategoryImage,

	// Áudio
	".mp3": CategoryAudio, ".wav": CategoryAudio, ".ogg": CategoryAudio,
	".flac": CategoryAudio, ".aac": CategoryAudio, ".m4a": CategoryAudio,
	".wma": CategoryAudio, ".aiff": CategoryAudio, ".opus": CategoryAudio,

	// Vídeo
	".mp4": CategoryVideo, ".avi": CategoryVideo, ".mkv": CategoryVideo,
	".mov": CategoryVideo, ".wmv": CategoryVideo, ".webm": CategoryVideo,
	".flv": CategoryVideo, ".m4v": CategoryVideo, ".mpeg": CategoryVideo,
	".mpg": CategoryVideo, ".3gp": CategoryVideo,

	// Arquivo/Compactado
	".zip": CategoryArchive, ".rar": CategoryArchive, ".7z": CategoryArchive,
	".tar": CategoryArchive, ".gz": CategoryArchive, ".bz2": CategoryArchive,
	".xz": CategoryArchive, ".tgz": CategoryArchive, ".tbz2": CategoryArchive,

	// Executável
	".exe": CategoryExecutable, ".dll": CategoryExecutable, ".so": CategoryExecutable,
	".dylib": CategoryExecutable, ".app": CategoryExecutable, ".msi": CategoryExecutable,
	".deb": CategoryExecutable, ".rpm": CategoryExecutable, ".apk": CategoryExecutable,
	".dmg": CategoryExecutable,

	// Dados
	".db": CategoryData, ".sqlite": CategoryData, ".sqlite3": CategoryData,
	".bak": CategoryData, ".mdb": CategoryData,
	".accdb": CategoryData, ".dat": CategoryData,
}

// Mapeamento de extensão para MIME type
var extensionMimeTypes = map[string]string{
	// Texto
	".txt": "text/plain", ".md": "text/markdown", ".csv": "text/csv",
	".html": "text/html", ".htm": "text/html", ".css": "text/css",
	".xml": "application/xml", ".json": "application/json",

	// Código
	".js": "application/javascript", ".ts": "application/typescript",
	".py": "text/x-python", ".go": "text/x-go", ".java": "text/x-java",

	// Documento
	".pdf": "application/pdf",
	".doc": "application/msword",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".xls": "application/vnd.ms-excel",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".ppt": "application/vnd.ms-powerpoint",
	".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	".epub": "application/epub+zip",
	".mobi": "application/x-mobipocket-ebook",

	// Imagem
	".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png",
	".gif": "image/gif", ".webp": "image/webp", ".svg": "image/svg+xml",
	".ico": "image/x-icon", ".bmp": "image/bmp",

	// Áudio
	".mp3": "audio/mpeg", ".wav": "audio/wav", ".ogg": "audio/ogg",
	".flac": "audio/flac", ".aac": "audio/aac", ".m4a": "audio/mp4",

	// Vídeo
	".mp4": "video/mp4", ".webm": "video/webm", ".avi": "video/x-msvideo",
	".mkv": "video/x-matroska", ".mov": "video/quicktime",

	// Arquivo
	".zip": "application/zip", ".rar": "application/x-rar-compressed",
	".7z": "application/x-7z-compressed", ".tar": "application/x-tar",
	".gz": "application/gzip",
}

// Categorias que são arquivos de texto
var textCategories = map[FileCategory]bool{
	CategoryText:   true,
	CategoryCode:   true,
	CategoryWeb:    true,
	CategoryConfig: true,
}

// DetectFileInfo detecta informações sobre um arquivo
func DetectFileInfo(path string) (*FileInfo, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	stat, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}

	ext := strings.ToLower(filepath.Ext(absPath))
	category := GetCategoryByExtension(ext)
	mimeType := GetMimeTypeByExtension(ext)

	info := &FileInfo{
		Path:        absPath,
		Name:        stat.Name(),
		Extension:   ext,
		Category:    category,
		MimeType:    mimeType,
		Size:        stat.Size(),
		SizeHuman:   FormatSize(stat.Size()),
		IsText:      IsTextCategory(category),
		IsBinary:    !IsTextCategory(category),
		IsDir:       stat.IsDir(),
		IsHidden:    isHidden(stat.Name()),
		IsReadOnly:  isReadOnly(stat),
		ModifiedAt:  stat.ModTime(),
		Permissions: stat.Mode().String(),
	}

	// Tenta obter data de criação (depende do OS)
	info.CreatedAt = getCreationTime(stat)

	return info, nil
}

// GetCategoryByExtension retorna a categoria de um arquivo pela extensão
func GetCategoryByExtension(ext string) FileCategory {
	ext = strings.ToLower(ext)
	if cat, ok := extensionCategories[ext]; ok {
		return cat
	}
	return CategoryUnknown
}

// GetMimeTypeByExtension retorna o MIME type pela extensão
func GetMimeTypeByExtension(ext string) string {
	ext = strings.ToLower(ext)
	if mime, ok := extensionMimeTypes[ext]; ok {
		return mime
	}
	return "application/octet-stream"
}

// IsTextCategory verifica se a categoria é de arquivo de texto
func IsTextCategory(cat FileCategory) bool {
	return textCategories[cat]
}

// IsTextFile verifica se um arquivo é de texto pela extensão
func IsTextFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	cat := GetCategoryByExtension(ext)
	return IsTextCategory(cat)
}

// FormatSize formata um tamanho em bytes para leitura humana
func FormatSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)

	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.2f TB", float64(bytes)/TB)
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d bytes", bytes)
	}
}

// ListDirectory lista o conteúdo de um diretório
func ListDirectory(path string, showHidden bool, filterType FileCategory) ([]DirEntry, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(absPath)
	if err != nil {
		return nil, err
	}

	var result []DirEntry
	for _, entry := range entries {
		name := entry.Name()

		// Pula arquivos ocultos se não solicitado
		if !showHidden && isHidden(name) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		ext := strings.ToLower(filepath.Ext(name))
		category := GetCategoryByExtension(ext)
		if entry.IsDir() {
			category = CategoryUnknown // Diretórios não têm categoria
		}

		// Aplica filtro de tipo
		if filterType != "" && filterType != CategoryUnknown {
			if !entry.IsDir() && category != filterType {
				continue
			}
		}

		result = append(result, DirEntry{
			Name:     name,
			Path:     filepath.Join(absPath, name),
			IsDir:    entry.IsDir(),
			Size:     info.Size(),
			Category: category,
			Modified: info.ModTime(),
		})
	}

	return result, nil
}

// isHidden verifica se um arquivo é oculto
func isHidden(name string) bool {
	if runtime.GOOS == "windows" {
		// No Windows, arquivos começando com . também são considerados ocultos
		return strings.HasPrefix(name, ".")
	}
	return strings.HasPrefix(name, ".")
}

// isReadOnly verifica se um arquivo é somente leitura
func isReadOnly(info os.FileInfo) bool {
	mode := info.Mode()
	return mode.Perm()&0200 == 0
}

// getCreationTime tenta obter a data de criação do arquivo
func getCreationTime(info os.FileInfo) time.Time {
	// Em Go padrão, não há acesso direto à data de criação
	return info.ModTime()
}

// GetCategoryIcon retorna um emoji para a categoria
func GetCategoryIcon(cat FileCategory) string {
	switch cat {
	case CategoryText:
		return "📄"
	case CategoryCode:
		return "💻"
	case CategoryWeb:
		return "🌐"
	case CategoryDocument:
		return "📝"
	case CategoryImage:
		return "🖼️"
	case CategoryAudio:
		return "🎵"
	case CategoryVideo:
		return "🎬"
	case CategoryArchive:
		return "📦"
	case CategoryExecutable:
		return "⚙️"
	case CategoryData:
		return "🗃️"
	case CategoryConfig:
		return "⚙️"
	default:
		return "📁"
	}
}

