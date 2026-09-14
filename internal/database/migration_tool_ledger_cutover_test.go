package database

import (
	"strings"
	"testing"
)

// TestValidateCutoverIgnoraOrfaoEmTabelaNaoLedger garante que órfãos
// PRÉ-EXISTENTES em tabelas fora do cutover (que o app cria fora deste pacote,
// ex.: chat_tabs/http_endpoints) não abortam a validação: o foreign_key_check
// é escopado às tabelas reconstruídas (chat_messages e job_runs).
func TestValidateCutoverIgnoraOrfaoEmTabelaNaoLedger(t *testing.T) {
	database := newMigratorTestDB(t)
	fullAutoMigrate(t, database)
	if err := database.Exec(`PRAGMA foreign_keys = OFF`).Error; err != nil {
		t.Fatal(err)
	}

	// Banco canônico limpo: a validação deve passar.
	if err := validateToolLedgerCutover(database); err != nil {
		t.Fatalf("validação de banco canônico limpo falhou: %v", err)
	}

	// Tabela não pertencente ao cutover, com órfão pré-existente.
	for _, statement := range []string{
		`CREATE TABLE legacy_probe (
			id text PRIMARY KEY,
			conversation_id text,
			CONSTRAINT fk_legacy_probe_conversation
				FOREIGN KEY (conversation_id) REFERENCES conversations(id)
		)`,
		`INSERT INTO legacy_probe (id, conversation_id) VALUES ('probe-1', 'conversa-inexistente')`,
	} {
		if err := database.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}

	// Um foreign_key_check GLOBAL enxerga o órfão (prova de que ele é real)…
	if got := queryCount(t, database, "SELECT COUNT(*) FROM pragma_foreign_key_check"); got == 0 {
		t.Fatal("órfão pré-existente não foi detectado pelo check global")
	}
	// …mas a validação escopada às tabelas do cutover deve ignorá-lo.
	if err := validateToolLedgerCutover(database); err != nil {
		t.Fatalf("órfão em tabela não-ledger abortou o cutover: %v", err)
	}
}

// TestValidateCutoverFalhaComOrfaoEmTabelaDoCutover garante que órfãos nas
// tabelas efetivamente reconstruídas (chat_messages, job_runs) continuam
// bloqueando a validação.
func TestValidateCutoverFalhaComOrfaoEmTabelaDoCutover(t *testing.T) {
	database := newMigratorTestDB(t)
	fullAutoMigrate(t, database)
	if err := database.Exec(`PRAGMA foreign_keys = OFF`).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`
		INSERT INTO chat_messages (id, conversation_id, role, content)
		VALUES ('msg-orfa', 'conversa-inexistente', 'user', 'oi')`).Error; err != nil {
		t.Fatal(err)
	}
	err := validateToolLedgerCutover(database)
	if err == nil {
		t.Fatal("órfão em chat_messages deveria abortar o cutover")
	}
	if !strings.Contains(err.Error(), "chat_messages") {
		t.Fatalf("erro deveria apontar chat_messages: %v", err)
	}
}
