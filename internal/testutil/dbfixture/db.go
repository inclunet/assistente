// Package dbfixture prepara bancos privados para testes de comportamento.
// Não deve ser usado para testar bootstrap, migrações ou upgrades publicados:
// esses testes precisam continuar chamando database.InitPath diretamente.
package dbfixture

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"assistente/internal/database"
)

var schema struct {
	once  sync.Once
	image []byte
	err   error
}

// Open cria um banco em diretório privado novo, retorna seu caminho absoluto,
// instala-o no singleton de database e restaura o singleton no cleanup.
// Como os demais usuários desse singleton, os testes consumidores não podem
// rodar em paralelo. Somente bytes do banco vazio são compartilhados: nunca
// conexão, transação, usuário ou dados de cenário. A imagem nasce novamente
// em cada processo de testes, pelo bootstrap real da revisão compilada.
func Open(t testing.TB) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join(t.TempDir(), "fixture.db"))
	if err != nil {
		t.Fatal(err)
	}
	schema.once.Do(func() {
		schema.image, schema.err = buildImage(filepath.Join(t.TempDir(), "schema.db"))
	})
	if schema.err != nil {
		t.Fatalf("preparar imagem de schema: %v", schema.err)
	}
	if err := os.WriteFile(path, schema.image, 0600); err != nil {
		t.Fatalf("copiar schema privado: %v", err)
	}
	restore := database.SetDB(nil)
	if err := database.OpenPath(path); err != nil {
		_ = database.Close()
		restore()
		t.Fatalf("abrir banco privado: %v", err)
	}
	sqlDB, err := database.DB().DB()
	if err != nil {
		_ = database.Close()
		restore()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		err := sqlDB.Close()
		restore()
		if err != nil {
			t.Errorf("fechar banco privado: %v", err)
		}
	})
	return path
}

func buildImage(path string) ([]byte, error) {
	restore := database.SetDB(nil)
	defer restore()
	if err := database.InitPath(path); err != nil {
		_ = database.Close()
		return nil, err
	}
	// Fechar todas as conexões consolida o WAL antes de copiar o arquivo.
	if err := database.Close(); err != nil {
		return nil, fmt.Errorf("fechar schema: %w", err)
	}
	return os.ReadFile(path)
}
