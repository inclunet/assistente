package filesystem

import (
	"crypto/sha256"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

var ErrFileVersionChanged = errors.New("file version changed")

// FileVersion é uma observação efêmera de um path, não uma autorização de acesso.
// Campos privados impedem que um DTO vindo da UI fabrique a versão observada.
type FileVersion struct {
	path  string
	info  fs.FileInfo
	hash  [sha256.Size]byte
	valid bool
}

// CaptureFileVersion observa identidade, metadados e conteúdo sem carregá-lo
// inteiro em memória. Symlinks devem ser resolvidos pela política do caller.
func CaptureFileVersion(path string) (FileVersion, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return FileVersion{}, err
	}
	v := FileVersion{path: filepath.Clean(absolute)}
	info, err := os.Lstat(v.path)
	if os.IsNotExist(err) {
		v.valid = true
		return v, nil
	}
	if err != nil {
		return FileVersion{}, err
	}
	if !info.Mode().IsRegular() {
		return FileVersion{}, ErrFileVersionChanged
	}
	f, err := os.Open(v.path)
	if err != nil {
		return FileVersion{}, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return FileVersion{}, err
	}
	if !sameFileObservation(info, opened) {
		return FileVersion{}, ErrFileVersionChanged
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return FileVersion{}, err
	}
	after, err := f.Stat()
	if err != nil {
		return FileVersion{}, err
	}
	current, err := os.Lstat(v.path)
	if err != nil {
		return FileVersion{}, err
	}
	if !sameFileObservation(opened, after) || !sameFileObservation(after, current) {
		return FileVersion{}, ErrFileVersionChanged
	}
	copy(v.hash[:], h.Sum(nil))
	v.info, v.valid = after, true
	return v, nil
}

func sameFileObservation(a, b fs.FileInfo) bool {
	return a != nil && b != nil && os.SameFile(a, b) && a.Size() == b.Size() &&
		a.Mode() == b.Mode() && a.ModTime().Equal(b.ModTime())
}

// Exists indica se a observação incluiu um arquivo existente.
func (v FileVersion) Exists() bool { return v.valid && v.info != nil }

// Validate compara uma observação antes de publicar um resultado de leitura.
// Não mantém lock após retornar nem autoriza o acesso ao arquivo.
func (v FileVersion) Validate(path string) error { return v.matches(path) }

func (v FileVersion) matches(path string) error {
	if !v.valid {
		return ErrFileVersionChanged
	}
	current, err := CaptureFileVersion(path)
	if err != nil {
		return err
	}
	if normalizeForComparison(v.path) != normalizeForComparison(current.path) ||
		(v.info == nil) != (current.info == nil) {
		return ErrFileVersionChanged
	}
	if v.info != nil && (!sameFileObservation(v.info, current.info) || v.hash != current.hash) {
		return ErrFileVersionChanged
	}
	return nil
}

// WriteFileBytesReplacingVersion recusa versões desatualizadas sob o lock dos
// escritores deste processo, inclusive após preparar/sincronizar o temporário.
// Criação usa publicação sem sobrescrita. Para arquivo existente, a checagem
// imediatamente anterior ao rename NÃO é CAS interprocessos: outro programa
// ainda pode escrever entre ambos. Não usar como fronteira contra adversários.
func WriteFileBytesReplacingVersion(path string, content []byte, perm fs.FileMode, expected FileVersion) error {
	unlock := lockFileMutation(path)
	defer unlock()
	if err := expected.matches(path); err != nil {
		return err
	}
	return writeFileBytesReplacing(path, content, perm, func(temp, target string) error {
		if err := expected.matches(target); err != nil {
			return err
		}
		if expected.Exists() {
			return os.Rename(temp, target)
		}
		// O temporário já está completo; Link falha se alguém publicou o alvo.
		// O defer do writer remove só o temporário, nunca o destino vencedor.
		if err := os.Link(temp, target); err != nil {
			if os.IsExist(err) {
				return ErrFileVersionChanged
			}
			return err
		}
		return nil
	})
}
