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

	// Search (FTS5)
	t.Run("SearchMessageContentWithContext", func(t *testing.T) {
		if _, err := SearchMessageContentWithContext(bg, "qualquer", 10); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})

	// Summary (rolling context)
	t.Run("GetConversationSummaryWithContext", func(t *testing.T) {
		if _, _, err := GetConversationSummaryWithContext(bg, "x"); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
	t.Run("UpdateConversationSummaryWithContext", func(t *testing.T) {
		if err := UpdateConversationSummaryWithContext(bg, "x", "summary", "msg"); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
	t.Run("SetSummarizingInProgressWithContext", func(t *testing.T) {
		if err := SetSummarizingInProgressWithContext(bg, "x", true); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
	t.Run("IsSummarizingInProgressWithContext", func(t *testing.T) {
		if _, err := IsSummarizingInProgressWithContext(bg, "x"); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
	t.Run("GetMessagesAfterIDWithContext", func(t *testing.T) {
		if _, err := GetMessagesAfterIDWithContext(bg, "x", ""); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})

	// Token stats (B11 — antes fail-open via scopedMessageQuery)
	t.Run("GetConversationDetailedTokenStatsWithContext", func(t *testing.T) {
		if _, err := GetConversationDetailedTokenStatsWithContext(bg, "x"); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
	t.Run("GetDetailedTokenStatsWithContext", func(t *testing.T) {
		if _, err := GetDetailedTokenStatsWithContext(bg, "x", ""); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
	t.Run("GetConversationTokenStatsWithContext", func(t *testing.T) {
		if _, err := GetConversationTokenStatsWithContext(bg, "x"); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
	t.Run("GetAllTokenStatsWithContext", func(t *testing.T) {
		if _, err := GetAllTokenStatsWithContext(bg); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
	t.Run("GetTurnTokenStatsWithContext", func(t *testing.T) {
		if _, err := GetTurnTokenStatsWithContext(bg, "x", "y"); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
	t.Run("GetContextWindowUsageWithContext", func(t *testing.T) {
		if _, _, err := GetContextWindowUsageWithContext(bg, "x", 4096); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
	t.Run("GetRecentMessagesTokenCountWithContext", func(t *testing.T) {
		if _, err := GetRecentMessagesTokenCountWithContext(bg, "x", 10); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})

	// Mensagens cross-conversation
	t.Run("GetMessagesBetweenIDsWithContext", func(t *testing.T) {
		if _, err := GetMessagesBetweenIDsWithContext(bg, "x", "a", "b"); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
	t.Run("GetMessageTreeWithContext", func(t *testing.T) {
		if _, _, err := GetMessageTreeWithContext(bg, "x"); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})

	// Search por título (Search Conversations Tool path)
	t.Run("SearchConversationsWithContext", func(t *testing.T) {
		if _, err := SearchConversationsWithContext(bg, "qualquer"); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})

	// HasMessageAudio: retorno é bool, valida que sem userID retorna false
	// sem tocar o banco em vez de vazar existência cross-user.
	t.Run("HasMessageAudioWithContext", func(t *testing.T) {
		if HasMessageAudioWithContext(bg, "qualquer-id") {
			t.Fatalf("HasMessageAudioWithContext sem userID deveria retornar false")
		}
	})

	// LLM Providers (vetor crítico — antes vazava credenciais)
	t.Run("GetLLMProvidersWithContext", func(t *testing.T) {
		if _, err := GetLLMProvidersWithContext(bg); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
	t.Run("GetLLMProviderWithContext", func(t *testing.T) {
		if _, err := GetLLMProviderWithContext(bg, "x"); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
	t.Run("DeleteLLMProviderWithContext", func(t *testing.T) {
		if err := DeleteLLMProviderWithContext(bg, "x"); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
	t.Run("CountLLMProvidersWithContext", func(t *testing.T) {
		if _, err := CountLLMProvidersWithContext(bg); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
	t.Run("SetDefaultProviderWithContext", func(t *testing.T) {
		if err := SetDefaultProviderWithContext(bg, "x"); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
	t.Run("GetDefaultProviderWithContext", func(t *testing.T) {
		if _, err := GetDefaultProviderWithContext(bg); !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("want ErrUserScopeRequired, got %v", err)
		}
	})
	// SaveLLMProviderWithContext aceita bootstrap como caminho legítimo;
	// sem userID e sem bootstrap, falha fechado.
	t.Run("SaveLLMProviderWithContext", func(t *testing.T) {
		err := SaveLLMProviderWithContext(bg, &LLMProvider{ID: "x", UserID: ""})
		if !errors.Is(err, ErrUserScopeRequired) {
			t.Fatalf("ctx vazio: want ErrUserScopeRequired, got %v", err)
		}
	})
	t.Run("SaveLLMProviderWithContext_BootstrapAccepted", func(t *testing.T) {
		bootstrap := WithBootstrap(bg)
		err := SaveLLMProviderWithContext(bootstrap, &LLMProvider{ID: "bootstrap-prov", UserID: ""})
		if err != nil {
			t.Fatalf("bootstrap legítimo: erro inesperado %v", err)
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
