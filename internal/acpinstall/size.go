package acpinstall

import (
	"context"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// sizeProbeTimeout é o prazo para descobrir o tamanho do artefato sem baixá-lo.
//
// O plano alimenta o diálogo de confirmação (D3): um HEAD lento não pode
// segurar a tela, e falha ou ausência de Content-Length só omite o tamanho —
// não bloqueia a instalação.
const sizeProbeTimeout = 3 * time.Second

// maxDiskWalkDepth é o teto de profundidade ao medir o diretório instalado.
// Instalação honesta não chega perto; o teto evita árvore patológica ou
// symlink que empurre o walk para o resto do disco.
const maxDiskWalkDepth = 24

// probeArtifactBytes pergunta o tamanho do download sem baixar o arquivo.
//
// Preferência: HEAD com Content-Length. Se HEAD falhar ou não informar, tenta
// GET com Range bytes=0-0 e lê o total em Content-Range (ou Content-Length numa
// resposta 200). Qualquer falha devolve 0 — a UI omite o campo (D3).
func probeArtifactBytes(ctx context.Context, client Doer, archiveURL string) int64 {
	if client == nil || strings.TrimSpace(archiveURL) == "" {
		return 0
	}
	probeCtx, cancel := context.WithTimeout(ctx, sizeProbeTimeout)
	defer cancel()

	if n := contentLengthOf(probeCtx, client, http.MethodHead, archiveURL, false); n > 0 {
		return n
	}
	return contentLengthOf(probeCtx, client, http.MethodGet, archiveURL, true)
}

// contentLengthOf faz um pedido e extrai o tamanho conhecido do artefato.
func contentLengthOf(ctx context.Context, client Doer, method, archiveURL string, withRange bool) int64 {
	req, err := http.NewRequestWithContext(ctx, method, archiveURL, nil)
	if err != nil {
		return 0
	}
	req.Header.Set("User-Agent", artifactUserAgent)
	if withRange {
		req.Header.Set("Range", "bytes=0-0")
	}
	resp, err := client.Do(ctx, req)
	if err != nil {
		return 0
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64))
		_ = resp.Body.Close()
	}()

	switch resp.StatusCode {
	case http.StatusOK:
		if resp.ContentLength > 0 {
			return resp.ContentLength
		}
	case http.StatusPartialContent:
		// Em 206, Content-Length é o tamanho do trecho (ex.: 1 byte para
		// Range bytes=0-0), não o do artefato. Só o total em Content-Range
		// serve; sem ele, omitimos (D3: "quando o servidor informa").
		return parseContentRangeTotal(resp.Header.Get("Content-Range"))
	}
	return 0
}

// parseContentRangeTotal lê o total de `Content-Range: bytes 0-0/12345`.
func parseContentRangeTotal(header string) int64 {
	header = strings.TrimSpace(header)
	if header == "" {
		return 0
	}
	i := strings.LastIndex(header, "/")
	if i < 0 || i+1 >= len(header) {
		return 0
	}
	total := strings.TrimSpace(header[i+1:])
	if total == "*" {
		return 0
	}
	n, err := strconv.ParseInt(total, 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// dirDiskBytes soma o tamanho dos arquivos sob dir, tolerando erros e cortando
// profundidade excessiva. Zero quando o diretório não existe ou está vazio.
func dirDiskBytes(dir string) int64 {
	if dir == "" {
		return 0
	}
	var total int64
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Entrada ilegível não derruba a medição: o tamanho parcial ainda
			// é melhor do que omitir o campo na tela por um arquivo travado.
			return nil
		}
		if path != dir {
			rel, relErr := filepath.Rel(dir, path)
			if relErr != nil {
				return nil
			}
			depth := strings.Count(rel, string(os.PathSeparator))
			if d.IsDir() && depth >= maxDiskWalkDepth {
				return fs.SkipDir
			}
		}
		if d.IsDir() {
			return nil
		}
		// Symlink: Info()/Stat seguiria o alvo e poderia somar arquivo fora da
		// instalação (e bem maior). Contamos só arquivo regular no próprio
		// diretório — a mesma disciplina da guarda de extração.
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total
}
