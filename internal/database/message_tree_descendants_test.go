package database

import (
	"context"
	"testing"
	"time"
)

// createMessageAt insere uma ChatMessage com ID e created_at explícitos,
// permitindo montar hierarquias determinísticas para os testes de descendentes.
func createMessageAt(t *testing.T, convID, id string, parentID *string, createdAt time.Time) {
	t.Helper()
	msg := ChatMessage{
		UUIDModel: UUIDModel{
			ID:        id,
			CreatedAt: createdAt,
		},
		ConversationID: convID,
		ParentID:       parentID,
		Role:           "assistant",
		Content:        "msg " + id,
	}
	if err := db.Create(&msg).Error; err != nil {
		t.Fatalf("failed to create message %s: %v", id, err)
	}
}

func strPtr(s string) *string { return &s }

// TestGetMessageTree_DeepHierarchyPreorder garante que a CTE recursiva retorna
// TODOS os descendentes de uma hierarquia profunda na ordem de uma travessia
// em pré-ordem (DFS), com os filhos de cada nível ordenados por created_at.
func TestGetMessageTree_DeepHierarchyPreorder(t *testing.T) {
	setupOrderingTestDB(t)

	conv := &Conversation{Title: "tree-deep", UserID: testUserID}
	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("failed to create conversation: %v", err)
	}

	base := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	rootID := "01980000-0000-7000-8000-00000000000r"

	// Árvore:
	//   R
	//   ├── A (t=1)
	//   │   ├── A1 (t=3)
	//   │   │   └── A1a (t=5)
	//   │   └── A2 (t=4)
	//   └── B (t=2)
	//       └── B1 (t=6)
	// Pré-ordem esperada a partir de R: A, A1, A1a, A2, B, B1
	createMessageAt(t, conv.ID, rootID, nil, base)
	// Inserção em ordem embaralhada de propósito — a ordenação efetiva deve
	// vir de created_at, não da ordem de inserção.
	createMessageAt(t, conv.ID, "B1", strPtr("B"), base.Add(6*time.Minute))
	createMessageAt(t, conv.ID, "A2", strPtr("A"), base.Add(4*time.Minute))
	createMessageAt(t, conv.ID, "B", strPtr(rootID), base.Add(2*time.Minute))
	createMessageAt(t, conv.ID, "A1a", strPtr("A1"), base.Add(5*time.Minute))
	createMessageAt(t, conv.ID, "A", strPtr(rootID), base.Add(1*time.Minute))
	createMessageAt(t, conv.ID, "A1", strPtr("A"), base.Add(3*time.Minute))

	message, descendants, err := GetMessageTreeWithContext(testCtx(), rootID)
	if err != nil {
		t.Fatalf("GetMessageTreeWithContext: %v", err)
	}
	if message.ID != rootID {
		t.Fatalf("expected root %s, got %s", rootID, message.ID)
	}

	want := []string{"A", "A1", "A1a", "A2", "B", "B1"}
	if len(descendants) != len(want) {
		t.Fatalf("expected %d descendants, got %d (%v)", len(want), len(descendants), idsOf(descendants))
	}
	for i, id := range want {
		if descendants[i].ID != id {
			t.Errorf("descendants[%d]: expected %s, got %s (full: %v)", i, id, descendants[i].ID, idsOf(descendants))
		}
	}
}

// TestGetMessageTree_SiblingTieBreakByID garante ordenação determinística
// quando irmãos compartilham o mesmo created_at (desempate por id ASC).
func TestGetMessageTree_SiblingTieBreakByID(t *testing.T) {
	setupOrderingTestDB(t)

	conv := &Conversation{Title: "tree-ties", UserID: testUserID}
	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("failed to create conversation: %v", err)
	}

	ts := time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC)
	rootID := "root-ties"
	createMessageAt(t, conv.ID, rootID, nil, ts)
	// Mesmo created_at, inseridos fora de ordem lexicográfica.
	createMessageAt(t, conv.ID, "child-c", strPtr(rootID), ts)
	createMessageAt(t, conv.ID, "child-a", strPtr(rootID), ts)
	createMessageAt(t, conv.ID, "child-b", strPtr(rootID), ts)

	_, descendants, err := GetMessageTreeWithContext(testCtx(), rootID)
	if err != nil {
		t.Fatalf("GetMessageTreeWithContext: %v", err)
	}
	want := []string{"child-a", "child-b", "child-c"}
	if len(descendants) != len(want) {
		t.Fatalf("expected %d descendants, got %d (%v)", len(want), len(descendants), idsOf(descendants))
	}
	for i, id := range want {
		if descendants[i].ID != id {
			t.Errorf("descendants[%d]: expected %s, got %s", i, id, descendants[i].ID)
		}
	}
}

// TestGetMessageTree_UserIsolation garante que descendentes pertencentes a
// outro usuário NUNCA aparecem na árvore — mesmo que uma mensagem de outro
// usuário tenha parent_id apontando para uma mensagem do usuário do contexto
// (estado adversarial). A CTE filtra por conversations.user_id em cada passo.
func TestGetMessageTree_UserIsolation(t *testing.T) {
	setupOrderingTestDB(t)

	base := time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC)

	// Conversa e árvore do usuário do contexto (testUserID).
	convA := &Conversation{Title: "owner", UserID: testUserID}
	if err := db.Create(convA).Error; err != nil {
		t.Fatalf("failed to create owner conversation: %v", err)
	}
	rootID := "iso-root"
	createMessageAt(t, convA.ID, rootID, nil, base)
	createMessageAt(t, convA.ID, "iso-child", strPtr(rootID), base.Add(1*time.Minute))

	// Conversa de outro usuário com uma mensagem cujo parent_id aponta para a
	// raiz e para o filho do usuário do contexto. Sem o filtro por user_id na
	// CTE, esses registros vazariam para a árvore.
	convB := &Conversation{Title: "intruder", UserID: "other-user"}
	if err := db.Create(convB).Error; err != nil {
		t.Fatalf("failed to create intruder conversation: %v", err)
	}
	createMessageAt(t, convB.ID, "leak-from-root", strPtr(rootID), base.Add(2*time.Minute))
	createMessageAt(t, convB.ID, "leak-from-child", strPtr("iso-child"), base.Add(3*time.Minute))

	_, descendants, err := GetMessageTreeWithContext(testCtx(), rootID)
	if err != nil {
		t.Fatalf("GetMessageTreeWithContext: %v", err)
	}

	want := []string{"iso-child"}
	if len(descendants) != len(want) {
		t.Fatalf("expected %d descendants, got %d (%v)", len(want), len(descendants), idsOf(descendants))
	}
	for _, d := range descendants {
		if d.ID == "leak-from-root" || d.ID == "leak-from-child" {
			t.Fatalf("user isolation violated: descendant %s of another user leaked", d.ID)
		}
	}

	// O outro usuário também não deve enxergar a árvore alheia ao consultar
	// pela mesma raiz (fail-closed retorna erro: a raiz não é dele).
	otherCtx := WithUserID(context.Background(), "other-user")
	if _, _, err := GetMessageTreeWithContext(otherCtx, rootID); err == nil {
		t.Fatalf("expected error when other user queries a root they do not own")
	}
}

func idsOf(msgs []ChatMessage) []string {
	ids := make([]string, len(msgs))
	for i, m := range msgs {
		ids[i] = m.ID
	}
	return ids
}
