package acpregistry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const (
	// cacheSubdir é onde o índice fica dentro de `.assistente/` (D2).
	cacheSubdir = "acp-registry"
	// cacheFileName é o arquivo do índice.
	cacheFileName = "registry.json"
	// cacheSchema versiona o envelope do cache — o carimbo e o índice cru —, e
	// não o documento do registro. Envelope de esquema desconhecido é tratado
	// como cache ausente.
	cacheSchema = 1
	// maxCacheBytes é o teto de leitura do arquivo. O índice de hoje tem cerca
	// de 48 KB; o teto existe para um arquivo trocado no disco não virar memória
	// gasta antes de qualquer validação.
	maxCacheBytes = 8 << 20
)

// As duas situações sem cache aproveitável levam ao mesmo lugar — buscar da
// rede —, mas só uma delas merece registro: nunca ter havido cache é o normal da
// primeira execução; ter cache que não se lê é um problema do disco.
var (
	errNoCache       = errors.New("sem cache do índice do registro ACP")
	errCacheUnusable = errors.New("cache do índice do registro ACP inaproveitável")
)

// cacheFile é o formato em disco: o carimbo de quando o índice foi coletado e o
// índice como ele foi gravado. O índice fica cru para a leitura passar pelo
// mesmo `ParseIndex` da resposta da rede — um arquivo adulterado não tem porta
// própria.
type cacheFile struct {
	Schema    int             `json:"schema"`
	FetchedAt time.Time       `json:"fetched_at"`
	Index     json.RawMessage `json:"index"`
}

// loadCache devolve o índice guardado e o carimbo da coleta.
func loadCache(ctx context.Context, path string) (Index, time.Time, error) {
	if path == "" {
		return Index{}, time.Time{}, errNoCache
	}
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Index{}, time.Time{}, errNoCache
		}
		return Index{}, time.Time{}, fmt.Errorf("%w: %v", errCacheUnusable, err)
	}
	defer func() { _ = file.Close() }()

	data, err := readAtMost(file, maxCacheBytes)
	if err != nil {
		return Index{}, time.Time{}, fmt.Errorf("%w: %v", errCacheUnusable, err)
	}

	var envelope cacheFile
	if err := json.Unmarshal(data, &envelope); err != nil {
		return Index{}, time.Time{}, fmt.Errorf("%w: arquivo ilegível: %v", errCacheUnusable, err)
	}
	if envelope.Schema != cacheSchema {
		return Index{}, time.Time{}, fmt.Errorf("%w: esquema %d desconhecido", errCacheUnusable, envelope.Schema)
	}
	if envelope.FetchedAt.IsZero() {
		// Sem carimbo não há idade a dizer, e dizer a idade é metade do valor de
		// servir cache velho (D2).
		return Index{}, time.Time{}, fmt.Errorf("%w: sem carimbo de coleta", errCacheUnusable)
	}

	index, err := ParseIndex(ctx, envelope.Index)
	if err != nil {
		return Index{}, time.Time{}, fmt.Errorf("%w: %v", errCacheUnusable, err)
	}
	return index, envelope.FetchedAt, nil
}

// saveCache grava o índice com o carimbo da coleta, de forma atômica: um app
// morto no meio da gravação deixa o cache anterior de pé, e não meio arquivo.
func saveCache(path string, index Index, fetchedAt time.Time) error {
	if path == "" {
		return errors.New("sem caminho para o cache do índice do registro ACP")
	}
	raw, err := json.Marshal(index)
	if err != nil {
		return fmt.Errorf("erro ao serializar o índice do registro ACP: %w", err)
	}
	data, err := json.MarshalIndent(cacheFile{Schema: cacheSchema, FetchedAt: fetchedAt, Index: raw}, "", "  ")
	if err != nil {
		return fmt.Errorf("erro ao serializar o cache do índice do registro ACP: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("erro ao criar o diretório do cache do índice do registro ACP: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".registry-*.tmp")
	if err != nil {
		return fmt.Errorf("erro ao criar arquivo temporário do cache do índice do registro ACP: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("erro ao gravar o cache do índice do registro ACP: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("erro ao finalizar o cache do índice do registro ACP: %w", err)
	}
	if err := replaceFile(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("erro ao substituir o cache do índice do registro ACP: %w", err)
	}
	return nil
}

// readAtMost lê até limit bytes e recusa o que passar disso, sem desserializar.
func readAtMost(r io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("conteúdo passou do limite de %d bytes", limit)
	}
	return data, nil
}

// replaceFile renomeia por cima do destino, repetindo em caso de falha
// passageira. No Windows, antivírus e indexador seguram o arquivo por um
// instante e o rename falha por acesso negado; em POSIX isso não acontece.
func replaceFile(tmpName, path string) error {
	const attempts = 5
	var err error
	for i := 0; i < attempts; i++ {
		if err = os.Rename(tmpName, path); err == nil {
			return nil
		}
		time.Sleep(time.Duration(i+1) * 10 * time.Millisecond)
	}
	return err
}
