package database

import (
	"context"
	"errors"
	"testing"
)

// TestDatabaseFunctionsRejectUnauthenticatedContext garante que toda função
// *WithContext escopada por user_id em internal/database/database.go falha
// fechado com ErrUserScopeRequired quando o ctx não carrega userID — ou seja,
// que a defesa não depende de cada caller chamar RequireUserID antes.
//
// Esse teste cobre o vetor de Major F do re-review do AEP-0052: hoje os
// testes _unauthenticated_test.go vivem em chat/tasklist/providers e cobrem
// só os DBStores; mas vários callers (app/db.go, gateway, summarization, etc.)
// chamam database.*WithContext direto, sem passar pelo DBStore. Esse caminho
// estava descoberto.
//
// Funções deliberadamente NÃO listadas:
//   - AdoptLegacyData / RebuildFTSIndex: instance-wide por design.
//   - FindOrCreateChannelConversationWithContext: bootstrap-tolerant, testado
//     separadamente (recebe WithBootstrap do gateway).
func TestDatabaseFunctionsRejectUnauthenticatedContext(t *testing.T) {
	setupUserScopeTestDB(t)

	bg := context.Background()

	// Conversation: writes
	t.Run("CreateConversationWithContext", func(t *testing.T) {
		if _, err := CreateConversationWithContext(bg, "x", ""); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
	t.Run("RecycleOrCreateConversationWithContext", func(t *testing.T) {
		if _, err := RecycleOrCreateConversationWithContext(bg, "x"); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
	t.Run("UpdateConversationWithContext", func(t *testing.T) {
		if err := UpdateConversationWithContext(bg, "x", "t", "m"); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
	t.Run("UpdateConversationChannelWithContext", func(t *testing.T) {
		if err := UpdateConversationChannelWithContext(bg, "x", "telegram", "123"); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
	t.Run("DeleteConversationWithContext", func(t *testing.T) {
		if err := DeleteConversationWithContext(bg, "x"); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})

	// Conversation: reads
	t.Run("GetConversationsWithContext", func(t *testing.T) {
		if _, err := GetConversationsWithContext(bg); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
	t.Run("GetConversationWithContext", func(t *testing.T) {
		if _, err := GetConversationWithContext(bg, "x"); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
	t.Run("GetConversationInfoWithContext", func(t *testing.T) {
		if _, err := GetConversationInfoWithContext(bg, "x"); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})

	// Message: writes
	t.Run("CreateMessageWithContext", func(t *testing.T) {
		if _, err := CreateMessageWithContext(bg, MessageOptions{ConversationID: "x", Role: "user", Content: "y"}); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
	t.Run("UpdateMessageContentWithContext", func(t *testing.T) {
		if err := UpdateMessageContentWithContext(bg, "x", "y", 0, 0, 0, ""); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
	t.Run("DeleteMessageWithContext", func(t *testing.T) {
		if err := DeleteMessageWithContext(bg, "x"); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
	t.Run("DeleteAllMessagesWithContext", func(t *testing.T) {
		if err := DeleteAllMessagesWithContext(bg, "x"); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
	t.Run("ClearAllConversationsWithContext", func(t *testing.T) {
		if err := ClearAllConversationsWithContext(bg); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
	t.Run("SaveMessageAudioWithContext", func(t *testing.T) {
		if err := SaveMessageAudioWithContext(bg, "x", "y", "audio/mpeg"); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})

	// Message: reads
	t.Run("GetMessageWithContext", func(t *testing.T) {
		if _, err := GetMessageWithContext(bg, "x"); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
	t.Run("GetMessageContentWithContext", func(t *testing.T) {
		if _, err := GetMessageContentWithContext(bg, "x"); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
	t.Run("GetMessageAudioWithContext", func(t *testing.T) {
		if _, _, err := GetMessageAudioWithContext(bg, "x"); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
	t.Run("GetMessagesWithContext", func(t *testing.T) {
		if _, err := GetMessagesWithContext(bg, "x", nil); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
	t.Run("GetRecentRootMessagesWithContext", func(t *testing.T) {
		if _, err := GetRecentRootMessagesWithContext(bg, "x", 10); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
	t.Run("GetRootMessagesBeforeWithContext", func(t *testing.T) {
		if _, err := GetRootMessagesBeforeWithContext(bg, "x", "y", 10); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
	t.Run("GetTurnMessagesWithContext", func(t *testing.T) {
		if _, err := GetTurnMessagesWithContext(bg, "x"); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
	t.Run("GetMessagesByTurnIDWithContext", func(t *testing.T) {
		if _, err := GetMessagesByTurnIDWithContext(bg, "x", nil, "y", 10); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
	t.Run("GetAllConversationMessagesWithContext", func(t *testing.T) {
		if _, err := GetAllConversationMessagesWithContext(bg, "x"); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
	t.Run("GetMessageWindowWithContext", func(t *testing.T) {
		if _, err := GetMessageWindowWithContext(bg, MessageWindowQuery{ConversationID: "x"}); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
	t.Run("CountChildrenWithContext", func(t *testing.T) {
		if _, err := CountChildrenWithContext(bg, []string{"x"}); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
}

// TestFindOrCreateChannelConversationWithContext_AcceptsBootstrap garante que
// o caminho bootstrap-tolerant continua funcionando: ctx marcado com
// WithBootstrap (mas sem userID) é aceito e produz conversa órfã. Sem
// nenhum dos dois (nem userID, nem bootstrap), retorna ErrUserScopeRequired.
func TestFindOrCreateChannelConversationWithContext_AcceptsBootstrap(t *testing.T) {
	setupUserScopeTestDB(t)

	bg := context.Background()
	if _, _, err := FindOrCreateChannelConversationWithContext(bg, "telegram", "123", "Fulano"); !errors.Is(err, ErrUserScopeRequired) {
		t.Fatalf("ctx vazio: want ErrUserScopeRequired, got %v", err)
	}

	bootstrapCtx := WithBootstrap(bg)
	conv, _, err := FindOrCreateChannelConversationWithContext(bootstrapCtx, "telegram", "123", "Fulano")
	if err != nil {
		t.Fatalf("ctx com WithBootstrap: erro inesperado %v", err)
	}
	if conv.UserID != "" {
		t.Fatalf("conversa bootstrap deveria nascer órfã (user_id=\"\"), got %q", conv.UserID)
	}
}
