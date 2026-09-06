package portability

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// legacyConversationsExportFile representa o contrato publicado até 0.1.9.
// Apesar de metadata.version ser "2.0", ele antecede o envelope numérico
// version=2 atual e usa IDs INTEGER e nomes snake_case no nível da conversa.
type legacyConversationsExportFile struct {
	Metadata struct {
		Version    string    `json:"version"`
		ExportedAt time.Time `json:"exported_at"`
		Type       string    `json:"type"`
	} `json:"metadata"`
	Conversations []legacyConversationExport `json:"conversations"`
}

type legacyConversationExport struct {
	ID        uint                  `json:"id"`
	Title     string                `json:"title"`
	CreatedAt time.Time             `json:"created_at"`
	Messages  []legacyMessageExport `json:"messages"`
}

type legacyMessageExport struct {
	ID               uint      `json:"id"`
	ParentID         *uint     `json:"parentId,omitempty"`
	TurnID           *uint     `json:"turnId,omitempty"`
	Role             string    `json:"role"`
	Content          string    `json:"content"`
	Reasoning        string    `json:"reasoning,omitempty"`
	Media            string    `json:"media,omitempty"`
	Audio            string    `json:"audio,omitempty"`
	AudioMimeType    string    `json:"audioMimeType,omitempty"`
	ToolCalls        string    `json:"toolCalls,omitempty"`
	ToolCallID       string    `json:"toolCallId,omitempty"`
	PromptTokens     int       `json:"promptTokens,omitempty"`
	CompletionTokens int       `json:"completionTokens,omitempty"`
	TotalTokens      int       `json:"totalTokens,omitempty"`
	Model            string    `json:"model,omitempty"`
	Source           string    `json:"source,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
}

const legacyExportNamespace = "db28819e-8584-59ec-9422-6f1c178d69dc"

func parseLegacyConversationsExport(raw []byte) (*ExportFile, bool, error) {
	var probe struct {
		Metadata struct {
			Version string `json:"version"`
			Type    string `json:"type"`
		} `json:"metadata"`
		Conversations json.RawMessage `json:"conversations"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, false, nil
	}
	if strings.TrimSpace(probe.Metadata.Type) != "conversations" ||
		strings.TrimSpace(probe.Metadata.Version) != "2.0" ||
		len(probe.Conversations) == 0 {
		return nil, false, nil
	}

	var legacy legacyConversationsExportFile
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return nil, true, fmt.Errorf("erro ao parsear exportação legada: %w", err)
	}

	file := &ExportFile{
		Version:    ExportVersion,
		ExportedAt: legacy.Metadata.ExportedAt,
		AppVersion: "0.1.9-ou-anterior",
		Options: ExportOptions{
			IncludeAudio: true,
		},
		Resources: ExportResources{
			Conversations: make([]ConversationExport, 0, len(legacy.Conversations)),
		},
	}

	for _, legacyConversation := range legacy.Conversations {
		conversationID := legacyStableID(
			legacy.Metadata.ExportedAt,
			"conversation",
			legacyConversation.ID,
			legacyConversation.Title,
		)
		messageIDs := make(map[uint]string, len(legacyConversation.Messages))
		for _, message := range legacyConversation.Messages {
			messageIDs[message.ID] = legacyStableID(
				legacy.Metadata.ExportedAt,
				"message:"+strconv.FormatUint(uint64(legacyConversation.ID), 10),
				message.ID,
				"",
			)
		}

		conversation := ConversationExport{
			ID:        conversationID,
			Title:     legacyConversation.Title,
			CreatedAt: legacyConversation.CreatedAt,
			Messages:  make([]MessageExport, 0, len(legacyConversation.Messages)),
		}
		for _, legacyMessage := range legacyConversation.Messages {
			message := MessageExport{
				ID:               messageIDs[legacyMessage.ID],
				ConversationID:   conversationID,
				Role:             legacyMessage.Role,
				Content:          legacyMessage.Content,
				Reasoning:        legacyMessage.Reasoning,
				Media:            legacyMessage.Media,
				Audio:            legacyMessage.Audio,
				AudioMimeType:    legacyMessage.AudioMimeType,
				ToolCalls:        legacyMessage.ToolCalls,
				ToolCallID:       legacyMessage.ToolCallID,
				PromptTokens:     legacyMessage.PromptTokens,
				CompletionTokens: legacyMessage.CompletionTokens,
				TotalTokens:      legacyMessage.TotalTokens,
				Model:            legacyMessage.Model,
				Source:           legacyMessage.Source,
				CreatedAt:        legacyMessage.CreatedAt,
			}
			if legacyMessage.ParentID != nil {
				message.ParentID = messageIDs[*legacyMessage.ParentID]
			}
			if legacyMessage.TurnID != nil {
				message.TurnID = messageIDs[*legacyMessage.TurnID]
			}
			conversation.Messages = append(conversation.Messages, message)
		}
		file.Resources.Conversations = append(file.Resources.Conversations, conversation)
	}

	return file, true, nil
}

func legacyStableID(exportedAt time.Time, kind string, oldID uint, discriminator string) string {
	key := exportedAt.UTC().Format(time.RFC3339Nano) + "|" + kind + "|" +
		strconv.FormatUint(uint64(oldID), 10) + "|" + discriminator
	hash := sha256.Sum256([]byte(legacyExportNamespace + "|" + key))
	var id uuid.UUID
	// UUIDv7 usa 48 bits de timestamp seguidos por versão/variante e entropia.
	// A entropia vem do hash estável para que reimportar o mesmo arquivo
	// preserve IDs, sem sair do contrato UUIDv7 usado pelo runtime.
	timestamp := exportedAt.UTC().UnixMilli()
	id[0] = byte(timestamp >> 40)
	id[1] = byte(timestamp >> 32)
	id[2] = byte(timestamp >> 24)
	id[3] = byte(timestamp >> 16)
	id[4] = byte(timestamp >> 8)
	id[5] = byte(timestamp)
	copy(id[6:], hash[:10])
	id[6] = (id[6] & 0x0f) | 0x70
	id[8] = (id[8] & 0x3f) | 0x80
	return id.String()
}
