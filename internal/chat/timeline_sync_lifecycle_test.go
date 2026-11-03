package chat

import (
	"reflect"
	"testing"

	"assistente/internal/database"
)

func TestSyncRequiredWindowRejectsReplacedExecutionOnOtherMutation(t *testing.T) {
	f, p, e, _ := automaticFixture(t)
	historicalMessage(t, f, "frontier", 1, nil)
	stream := e.StartStream(0)
	stream.Append("preservar")
	sub, initial := membershipSubscribe(t, f, p, "reading", 10)
	c := e.conversation
	if err := p.ApplyStorageMutation(f.ctx, f.conversation, []string{"message:frontier"}, func() ([]string, error) {
		return nil, database.DeleteMessageWithContext(f.ctx, "frontier")
	}); err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	w := c.windows[sub.windowID]
	invalid := w.syncRequired && reflect.DeepEqual(w.orderedIDs, initial.OrderedItemIDs)
	if c.publicationTimer != nil {
		c.publicationTimer.Stop()
		c.publicationTimer = nil
		c.publicationGeneration++
	}
	c.mu.Unlock()
	if !invalid {
		t.Fatal("perda de fronteira não preservou janela aguardando sincronização")
	}
	repo := database.NewChatTurnRepository(f.db)
	if err := repo.Finish(f.ctx, f.conversation, f.question, e.executionID, database.TurnInterrupted, "cancelled"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Begin(f.ctx, f.conversation, f.question, "", database.TurnModeRetry); err != nil {
		t.Fatal(err)
	}
	// A escrita não contém IDs do turno substituído nem publica a janela inválida.
	membershipWrite(t, f, p, "other", 30)
	if stream.Append("tardio") {
		t.Fatal("janela syncRequired manteve a execução substituída vinculada")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	w = c.windows[sub.windowID]
	turn := c.turns[e.turnID]
	if !w.syncRequired || !reflect.DeepEqual(w.orderedIDs, initial.OrderedItemIDs) {
		t.Fatal("prova de lifecycle reativou ou substituiu a janela inválida")
	}
	if !turn.detached || turn.streams[0].parts[0].text.String() != "preservar" {
		t.Fatal("desvinculação perdeu ou alterou o buffer anterior")
	}
}
