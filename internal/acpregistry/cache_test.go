package acpregistry

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOCacheGuardaOCarimboDaColeta(t *testing.T) {
	index, err := ParseIndex(context.Background(), []byte(indiceBom))
	if err != nil {
		t.Fatalf("ParseIndex devolveu erro: %v", err)
	}
	caminho := filepath.Join(t.TempDir(), cacheFileName)
	coleta := time.Date(2026, 8, 6, 9, 30, 0, 0, time.UTC)

	if err := saveCache(caminho, index, coleta); err != nil {
		t.Fatalf("saveCache devolveu erro: %v", err)
	}

	lido, carimbo, err := loadCache(context.Background(), caminho)
	if err != nil {
		t.Fatalf("loadCache devolveu erro: %v", err)
	}
	if !carimbo.Equal(coleta) {
		t.Errorf("carimbo = %v, quer %v", carimbo, coleta)
	}
	if lido.Version != index.Version || len(lido.Agents) != len(index.Agents) {
		t.Errorf("índice lido = versão %q com %d agentes", lido.Version, len(lido.Agents))
	}
	// O alvo binário volta inteiro: é dele que a Fase 4 tira o que baixar.
	alvo := agentePorID(t, lido.Agents, "goose").Distribution.Binary["windows-x86_64"]
	if alvo.SHA256 != strings.ToLower(digestDeTeste) || alvo.Cmd == "" {
		t.Errorf("alvo lido = %+v", alvo)
	}
}

func TestCacheAusenteNaoEProblemaDeDisco(t *testing.T) {
	caminho := filepath.Join(t.TempDir(), cacheFileName)
	_, _, err := loadCache(context.Background(), caminho)
	if !errors.Is(err, errNoCache) {
		t.Fatalf("erro = %v, quer errNoCache", err)
	}
	if errors.Is(err, errCacheUnusable) {
		t.Error("cache que nunca existiu não deveria ser reportado como inaproveitável")
	}
}

func TestSemCaminhoNaoHaCacheNemGravacao(t *testing.T) {
	if _, _, err := loadCache(context.Background(), ""); !errors.Is(err, errNoCache) {
		t.Errorf("loadCache(\"\") = %v, quer errNoCache", err)
	}
	if err := saveCache("", Index{Version: "1.0.0"}, time.Now()); err == nil {
		t.Error("saveCache(\"\") não deveria dizer que gravou")
	}
}

func TestOCacheInaproveitavelEhTratadoComoAusente(t *testing.T) {
	casos := map[string]string{
		"não é json":            "{isso não é json",
		"esquema desconhecido":  `{"schema":99,"fetched_at":"2026-08-06T09:30:00Z","index":{"version":"1.0.0","agents":[{"id":"a","name":"A","distribution":{"npx":{"package":"a"}}}]}}`,
		"sem carimbo":           `{"schema":1,"index":{"version":"1.0.0","agents":[{"id":"a","name":"A","distribution":{"npx":{"package":"a"}}}]}}`,
		"índice de outro major": `{"schema":1,"fetched_at":"2026-08-06T09:30:00Z","index":{"version":"2.0.0","agents":[{"id":"a","name":"A","distribution":{"npx":{"package":"a"}}}]}}`,
		"índice sem agentes":    `{"schema":1,"fetched_at":"2026-08-06T09:30:00Z","index":{"version":"1.0.0","agents":[]}}`,
		"índice truncado":       `{"schema":1,"fetched_at":"2026-08-06T09:30:00Z","index":{"version":"1.0.0","agents":[{"id":"a"`,
	}
	for nome, conteudo := range casos {
		t.Run(nome, func(t *testing.T) {
			caminho := filepath.Join(t.TempDir(), cacheFileName)
			if err := os.WriteFile(caminho, []byte(conteudo), 0o600); err != nil {
				t.Fatalf("não foi possível preparar o arquivo: %v", err)
			}
			_, carimbo, err := loadCache(context.Background(), caminho)
			if !errors.Is(err, errCacheUnusable) {
				t.Fatalf("erro = %v, quer errCacheUnusable", err)
			}
			if !carimbo.IsZero() {
				t.Errorf("carimbo = %v, quer zero", carimbo)
			}
		})
	}
}

// O arquivo do cache passa pela mesma validação da resposta da rede: um índice
// adulterado no disco não tem porta própria de entrada.
func TestOTextoDoCacheTambemEhSaneadoNaLeitura(t *testing.T) {
	caminho := filepath.Join(t.TempDir(), cacheFileName)
	conteudo := `{"schema":1,"fetched_at":"2026-08-06T09:30:00Z","index":` + indiceComTextoMalicioso + `}`
	if err := os.WriteFile(caminho, []byte(conteudo), 0o600); err != nil {
		t.Fatalf("não foi possível preparar o arquivo: %v", err)
	}

	index, _, err := loadCache(context.Background(), caminho)
	if err != nil {
		t.Fatalf("loadCache devolveu erro: %v", err)
	}
	agente := agentePorID(t, index.Agents, "sujo")
	if agente.Name != "Agente" {
		t.Errorf("nome = %q, quer saneado", agente.Name)
	}
	if agente.Website != "" {
		t.Errorf("website = %q, quer vazio", agente.Website)
	}
}

// Escrita atômica: o arquivo temporário ou virou o cache ou foi embora, e a
// segunda gravação não deixa resíduo no diretório.
func TestAGravacaoDoCacheNaoDeixaTemporarioParaTras(t *testing.T) {
	dir := t.TempDir()
	caminho := filepath.Join(dir, cacheFileName)
	index, err := ParseIndex(context.Background(), []byte(indiceBom))
	if err != nil {
		t.Fatalf("ParseIndex devolveu erro: %v", err)
	}

	for i := 0; i < 3; i++ {
		if err := saveCache(caminho, index, time.Now()); err != nil {
			t.Fatalf("saveCache devolveu erro: %v", err)
		}
	}

	entradas, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("não foi possível listar o diretório: %v", err)
	}
	if len(entradas) != 1 || entradas[0].Name() != cacheFileName {
		nomes := make([]string, 0, len(entradas))
		for _, entrada := range entradas {
			nomes = append(nomes, entrada.Name())
		}
		t.Errorf("diretório = %v, quer só o %s", nomes, cacheFileName)
	}
}

func TestOCacheEhGravadoComOEsquemaDoEnvelope(t *testing.T) {
	caminho := filepath.Join(t.TempDir(), cacheFileName)
	index, err := ParseIndex(context.Background(), []byte(indiceBom))
	if err != nil {
		t.Fatalf("ParseIndex devolveu erro: %v", err)
	}
	if err := saveCache(caminho, index, time.Now()); err != nil {
		t.Fatalf("saveCache devolveu erro: %v", err)
	}

	data, err := os.ReadFile(caminho)
	if err != nil {
		t.Fatalf("não foi possível ler o cache: %v", err)
	}
	var envelope cacheFile
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("o cache gravado não é json: %v", err)
	}
	if envelope.Schema != cacheSchema {
		t.Errorf("esquema = %d, quer %d", envelope.Schema, cacheSchema)
	}
	if envelope.FetchedAt.IsZero() || len(envelope.Index) == 0 {
		t.Errorf("envelope = %+v", envelope)
	}
}

// Um arquivo grande demais é recusado sem ser desserializado.
func TestOCacheGiganteEhRecusado(t *testing.T) {
	caminho := filepath.Join(t.TempDir(), cacheFileName)
	gordura := make([]byte, maxCacheBytes+1)
	for i := range gordura {
		gordura[i] = 'a'
	}
	if err := os.WriteFile(caminho, gordura, 0o600); err != nil {
		t.Fatalf("não foi possível preparar o arquivo: %v", err)
	}
	if _, _, err := loadCache(context.Background(), caminho); !errors.Is(err, errCacheUnusable) {
		t.Fatalf("erro = %v, quer errCacheUnusable", err)
	}
}
