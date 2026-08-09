package app

import (
	"context"
	"testing"

	"assistente/internal/credentials"
	"assistente/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupACPCredentialTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("falha ao criar banco em memória: %v", err)
	}
	if err := db.AutoMigrate(&database.CredentialEntry{}, &database.CredentialKeyWrap{}); err != nil {
		t.Fatalf("falha ao migrar tabelas: %v", err)
	}
	database.SetDB(db)
}

// O valor que vai para o ambiente do agente sai do cofre, e sai decifrado: é a
// ponta do AEP-0086 D12 que liga o provedor ao segredo guardado.
func TestACredencialDoAgenteSaiDoCofreDecifrada(t *testing.T) {
	setupACPCredentialTestDB(t)
	ctx := database.WithUserID(context.Background(), "u1")

	credMgr := credentials.NewManagerWithStoreAndPersistence(
		[]byte("test-key-exactly-32-bytes-long!!"), credentials.NewDBStore(), true)
	if err := credMgr.RegisterPatternWithContext(ctx, "api.openai.com", &credentials.AuthConfig{
		Type: "bearer", Token: "sk-do-cofre",
	}); err != nil {
		t.Fatalf("registrar a credencial: %v", err)
	}
	a := &App{credMgr: credMgr}

	valor, err := a.acpCredentialFromVault(ctx, "api.openai.com")
	if err != nil {
		t.Fatalf("ler do cofre: %v", err)
	}
	if valor != "sk-do-cofre" {
		t.Errorf("valor = %q, esperado o do cofre", valor)
	}
}

// Entrada que não existe é vazio, e não erro: quem sabe transformar isso em
// recusa do spawn é o manager, que conhece a variável e o provider e consegue
// dizer o que falta (AEP-0086 D12).
func TestEntradaAusenteNoCofreVoltaVaziaSemErro(t *testing.T) {
	setupACPCredentialTestDB(t)
	ctx := database.WithUserID(context.Background(), "u1")

	a := &App{credMgr: credentials.NewManagerWithStoreAndPersistence(
		[]byte("test-key-exactly-32-bytes-long!!"), credentials.NewDBStore(), true)}

	valor, err := a.acpCredentialFromVault(ctx, "api.openai.com")
	if err != nil {
		t.Fatalf("ler do cofre: %v", err)
	}
	if valor != "" {
		t.Errorf("valor = %q, esperado vazio", valor)
	}
}

// O cofre é escopado por usuário (AEP-0052): a credencial de uma pessoa não
// pode subir no agente de outra.
func TestOAgenteDeUmaPessoaNaoRecebeACredencialDeOutra(t *testing.T) {
	setupACPCredentialTestDB(t)
	daAna := database.WithUserID(context.Background(), "ana")
	doLeo := database.WithUserID(context.Background(), "leo")

	credMgr := credentials.NewManagerWithStoreAndPersistence(
		[]byte("test-key-exactly-32-bytes-long!!"), credentials.NewDBStore(), true)
	if err := credMgr.RegisterPatternWithContext(daAna, "api.openai.com", &credentials.AuthConfig{
		Type: "bearer", Token: "sk-da-ana",
	}); err != nil {
		t.Fatalf("registrar a credencial: %v", err)
	}
	a := &App{credMgr: credMgr}

	valor, err := a.acpCredentialFromVault(doLeo, "api.openai.com")
	if err != nil {
		t.Fatalf("ler do cofre: %v", err)
	}
	if valor != "" {
		t.Errorf("valor = %q, esperado nada: a entrada é de outra pessoa", valor)
	}
}

// Sem cofre não há como cumprir a promessa, e dizer isso é melhor do que
// devolver vazio: vazio viraria "a entrada não existe", que manda a pessoa
// cadastrar de novo o que já está lá.
func TestSemCofreALeituraDizQueOCofreFalta(t *testing.T) {
	a := &App{}
	if _, err := a.acpCredentialFromVault(context.Background(), "api.openai.com"); err == nil {
		t.Fatal("esperava erro dizendo que o cofre não está disponível")
	}
}
