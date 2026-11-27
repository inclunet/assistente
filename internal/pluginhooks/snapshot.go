package pluginhooks

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// writeSnapshot materializa somente arquivos da revisão, numa pasta nova e
// privada. O_EXCL também rejeita aliases e colisões de caixa no Windows.
func writeSnapshot(ctx context.Context, dir string, files map[string][]byte) error {
	if len(files) > 1024 {
		return fs.ErrInvalid
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	total := 0
	for name, body := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !fs.ValidPath(name) || name == "." || strings.ContainsAny(name, `\:`) {
			return fs.ErrInvalid
		}
		local, err := filepath.Localize(name)
		if err != nil {
			return err
		}
		if len(body) > 16<<20-total {
			return fs.ErrInvalid
		}
		total += len(body)
		if err = root.MkdirAll(filepath.Dir(local), 0700); err != nil {
			return err
		}
		file, err := root.OpenFile(local, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, err = file.Write(body)
		if err = errors.Join(err, file.Close()); err != nil {
			return err
		}
	}
	return nil
}
