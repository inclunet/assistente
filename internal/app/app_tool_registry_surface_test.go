package app

import (
	"context"
	"testing"

	"assistente/internal/database"
	"assistente/internal/eventctx"
	"assistente/internal/questionnaire"
)

func TestResolveProfileAccessSurfaceSkipsLookupWithoutInterlocutor(t *testing.T) {
	tests := []struct {
		name           string
		ctx            context.Context
		source         string
		conversationID string
	}{
		{name: "job por proveniência canônica", ctx: eventctx.With(context.Background(), eventctx.Provenance{Source: "job"}), source: "wails", conversationID: "conv-1"},
		{name: "job explícito", ctx: context.Background(), source: "job", conversationID: "conv-1"},
		{name: "system", ctx: context.Background(), source: "system", conversationID: "conv-1"},
		{name: "conversa vazia", ctx: context.Background(), source: "telegram", conversationID: "  "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lookups := 0
			surface := resolveProfileAccessSurface(tt.ctx, tt.source, tt.conversationID, func(context.Context, string) (*database.Conversation, error) {
				lookups++
				return &database.Conversation{Channel: "telegram", ContactID: "contact-1"}, nil
			})
			if surface.Kind != questionnaire.SurfaceNone {
				t.Fatalf("surface = %#v, esperava NoSurface", surface)
			}
			if lookups != 0 {
				t.Fatalf("lookup SQLite chamado %d vez(es), esperava zero", lookups)
			}
		})
	}
}

func TestResolveProfileAccessSurfacePreservesInteractiveOrigins(t *testing.T) {
	lookups := 0
	lookup := func(context.Context, string) (*database.Conversation, error) {
		lookups++
		return &database.Conversation{Channel: "telegram", ContactID: "contact-1"}, nil
	}

	desktop := resolveProfileAccessSurface(context.Background(), "wails", "conv-desktop", lookup)
	if desktop.Kind != questionnaire.SurfaceDesktop || desktop.ConversationID != "conv-desktop" {
		t.Fatalf("desktop = %#v", desktop)
	}
	if lookups != 0 {
		t.Fatalf("desktop consultou SQLite %d vez(es)", lookups)
	}

	channel := resolveProfileAccessSurface(context.Background(), "telegram", "conv-channel", lookup)
	if channel.Kind != questionnaire.SurfaceChannel || channel.Channel != "telegram" || channel.ContactID != "contact-1" {
		t.Fatalf("channel = %#v", channel)
	}
	if lookups != 1 {
		t.Fatalf("canal consultou SQLite %d vez(es), esperava uma", lookups)
	}
}
