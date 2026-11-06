package controllers

import (
	"reflect"
	"testing"
)

// A apresentação deve passar pelas capabilities da timeline. O controller CRUD
// não pode voltar a expor leitura avulsa que contorne essa autorização/barreira.
func TestConversationControllerDoesNotExposePresentationReaders(t *testing.T) {
	controller := reflect.TypeOf(NewConversationsController(ConversationsControllerConfig{}))
	for _, name := range []string{"GetMessages", "GetRecentMessages", "GetMessagesBefore", "GetConversationMessageWindow", "GetConversationWithThreads", "GetMessageChildren"} {
		if _, found := controller.MethodByName(name); found {
			t.Errorf("leitor paralelo: %s", name)
		}
	}
}
func TestConversationControllerRequiresNoLegacyReader(t *testing.T) {
	config := reflect.TypeOf(ConversationsControllerConfig{})
	if _, found := config.FieldByName("ReadTimelineWindow"); found {
		t.Fatal("dependência visual legada presente")
	}
}
