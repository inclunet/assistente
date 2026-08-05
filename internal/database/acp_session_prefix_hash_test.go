package database

import (
	"testing"

	"gorm.io/gorm"
)

func temColunaDePrefixo(t *testing.T, database *gorm.DB) bool {
	t.Helper()
	var total int64
	if err := database.Raw(`SELECT COUNT(*) FROM pragma_table_info('acp_sessions') WHERE name = 'prompt_prefix_hash'`).Scan(&total).Error; err != nil {
		t.Fatalf("procurar a coluna prompt_prefix_hash: %v", err)
	}
	return total > 0
}

// A coluna guardava o prefixo de perfil que a sessão já tinha ouvido. O app não
// manda mais instrução nenhuma ao agente (AEP-0084, Fase 8), então ela sai — e
// o vínculo conversa↔sessão, que é o que importa na tabela, fica de pé.
func TestBaseAntigaPerdeAColunaDePrefixoESegueUtilizavel(t *testing.T) {
	database := newMigratorTestDB(t)
	criaAcpSessionsLegado(t, database)
	insereSessaoLegada(t, database, "a1", "user-ana", "conv-1", "cursor", "sess-1", "2026-01-01")

	if !temColunaDePrefixo(t, database) {
		t.Fatal("o schema legado do teste precisa nascer com a coluna")
	}
	if err := bootaSobre(t, database); err != nil {
		t.Fatalf("boot sobre base antiga: %v", err)
	}
	if temColunaDePrefixo(t, database) {
		t.Error("a coluna prompt_prefix_hash continuou na tabela")
	}

	lidas := leSessoes(t, database)
	if len(lidas) != 1 || lidas[0].SessionID != "sess-1" || lidas[0].UserID != "user-ana" {
		t.Fatalf("o vínculo da sessão não sobreviveu ao drop: %+v", lidas)
	}

	nova := ACPSession{
		UserID:         "user-ana",
		ConversationID: "conv-2",
		ProviderID:     "cursor",
		SessionID:      "sess-2",
		Cwd:            "/projeto",
	}
	if err := database.Create(&nova).Error; err != nil {
		t.Fatalf("gravar sessão depois do drop: %v", err)
	}
}

// Banco novo nasce sem a coluna: a migração não pode reclamar de não ter o que
// dropar, nem ficar se retentando a cada boot.
func TestBaseNovaPassaPelaMigracaoDoPrefixoSemNadaAFazer(t *testing.T) {
	database := newMigratorTestDB(t)

	if err := bootaSobre(t, database); err != nil {
		t.Fatalf("primeiro boot: %v", err)
	}
	if temColunaDePrefixo(t, database) {
		t.Fatal("banco novo nasceu com a coluna prompt_prefix_hash")
	}
	if err := bootaSobre(t, database); err != nil {
		t.Fatalf("segundo boot: %v", err)
	}
}
