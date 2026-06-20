package profiles

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Bug 2: GetActive auto-cura múltiplos active=true.
// Cenário: o user tinha `padrao.json` e `programacao.json` ambos com
// active=true (efeito colateral do builtin re-instalando active=true a
// cada upgrade). GetActive precisa escolher determinísticamente o mais
// recente (mtime) e desativar os demais no disco.
func TestGetActive_AutoCuraMultiplosActive(t *testing.T) {
	manager := setupProfileTestEnv(t)

	older := DefaultProfile()
	older.Name = "Padrão"
	older.Active = true
	if _, err := manager.Create(older); err != nil {
		t.Fatalf("create older: %v", err)
	}

	newer := DefaultProfile()
	newer.Name = "Programação"
	newer.Active = true
	if _, err := manager.Create(newer); err != nil {
		t.Fatalf("create newer: %v", err)
	}

	// Garante mtime distinto independentemente da resolução do FS.
	homeDir := manager.resolver.GetHomeDir()
	older2hAgo := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(filepath.Join(homeDir, "padrao.json"), older2hAgo, older2hAgo); err != nil {
		t.Fatalf("chtimes padrao: %v", err)
	}

	active, err := manager.GetActive()
	if err != nil {
		t.Fatalf("GetActive: %v", err)
	}
	if active.Name != "Programação" {
		t.Errorf("GetActive escolheu %q; esperado %q (mais recente)", active.Name, "Programação")
	}

	// Auto-cura: o "perdedor" foi marcado inativo no disco. Sem isso, a
	// próxima leitura veria de novo dois active=true e o user continuaria
	// sentindo o picker sair do controle.
	loserData, err := os.ReadFile(filepath.Join(homeDir, "padrao.json"))
	if err != nil {
		t.Fatalf("re-read padrao: %v", err)
	}
	var loser Profile
	if err := json.Unmarshal(loserData, &loser); err != nil {
		t.Fatalf("unmarshal loser: %v", err)
	}
	if loser.Active {
		t.Error("padrao.json deveria ter sido desativado pela auto-cura")
	}
}

// GetActiveSlug delega para resolveActive (mesma resolução de GetActive),
// inclusive a auto-cura por mtime. Este teste garante que: (1) o slug retornado
// é o do vencedor determinístico (mais recente) e (2) a auto-cura persiste os
// demais perfis como inativos no disco — o efeito colateral de I/O documentado.
func TestGetActiveSlug_AutoCuraMultiplosActive(t *testing.T) {
	manager := setupProfileTestEnv(t)

	older := DefaultProfile()
	older.Name = "Padrão"
	older.Active = true
	if _, err := manager.Create(older); err != nil {
		t.Fatalf("create older: %v", err)
	}

	newer := DefaultProfile()
	newer.Name = "Programação"
	newer.Active = true
	if _, err := manager.Create(newer); err != nil {
		t.Fatalf("create newer: %v", err)
	}

	// Garante mtime distinto: padrao.json fica mais antigo que programacao.json.
	homeDir := manager.resolver.GetHomeDir()
	older2hAgo := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(filepath.Join(homeDir, "padrao.json"), older2hAgo, older2hAgo); err != nil {
		t.Fatalf("chtimes padrao: %v", err)
	}

	slug := manager.GetActiveSlug()
	if slug != "programacao" {
		t.Errorf("GetActiveSlug devolveu %q; esperado %q (mais recente)", slug, "programacao")
	}

	// Auto-cura: o perdedor (padrao) deve ter sido regravado como inativo.
	loserData, err := os.ReadFile(filepath.Join(homeDir, "padrao.json"))
	if err != nil {
		t.Fatalf("re-read padrao: %v", err)
	}
	var loser Profile
	if err := json.Unmarshal(loserData, &loser); err != nil {
		t.Fatalf("unmarshal loser: %v", err)
	}
	if loser.Active {
		t.Error("padrao.json deveria ter sido desativado pela auto-cura via GetActiveSlug")
	}
}

// Bug 3: Update com Active=true deve enforçar unicidade desativando os outros.
func TestUpdate_AtivarUmDesativaOutros(t *testing.T) {
	manager := setupProfileTestEnv(t)

	a := DefaultProfile()
	a.Name = "Alpha"
	a.Active = true
	if _, err := manager.Create(a); err != nil {
		t.Fatalf("create a: %v", err)
	}

	b := DefaultProfile()
	b.Name = "Beta"
	b.Active = false
	if _, err := manager.Create(b); err != nil {
		t.Fatalf("create b: %v", err)
	}

	// Update Beta marcando-o como ativo. Update DEVE desativar Alpha.
	bUpdated, err := manager.Get("beta")
	if err != nil {
		t.Fatalf("get beta: %v", err)
	}
	bUpdated.Active = true
	if err := manager.Update("beta", bUpdated); err != nil {
		t.Fatalf("update beta: %v", err)
	}

	a2, err := manager.Get("alpha")
	if err != nil {
		t.Fatalf("get alpha: %v", err)
	}
	if a2.Active {
		t.Error("Update(beta, Active=true) deveria ter desativado Alpha — invariante de unicidade violada")
	}
	b2, err := manager.Get("beta")
	if err != nil {
		t.Fatalf("get beta v2: %v", err)
	}
	if !b2.Active {
		t.Error("Beta deveria estar ativo após Update")
	}
}

// Bug 3 + 2: depois do Update enforce, GetActive precisa devolver o ativo único.
func TestUpdate_DepoisGetActiveDevolveCertoSemAutocura(t *testing.T) {
	manager := setupProfileTestEnv(t)

	for _, name := range []string{"Padrão", "Programação", "Modelo Local"} {
		p := DefaultProfile()
		p.Name = name
		p.Active = (name == "Padrão")
		if _, err := manager.Create(p); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}

	modelo, _ := manager.Get("modelo-local")
	modelo.Active = true
	if err := manager.Update("modelo-local", modelo); err != nil {
		t.Fatalf("update modelo: %v", err)
	}

	active, err := manager.GetActive()
	if err != nil {
		t.Fatalf("GetActive: %v", err)
	}
	if active.Name != "Modelo Local" {
		t.Errorf("GetActive devolveu %q; esperado %q", active.Name, "Modelo Local")
	}
}
