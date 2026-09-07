package controllers

import (
	"testing"

	"assistente/internal/database"
)

func TestNormalizeConversationPageRequest(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		limit      int
		offset     int
		wantLimit  int
		wantOffset int
	}{
		{
			name:       "preserva sem paginacao",
			limit:      0,
			offset:     0,
			wantLimit:  0,
			wantOffset: 0,
		},
		{
			name:       "normaliza offset antes de aplicar default",
			limit:      0,
			offset:     -1,
			wantLimit:  0,
			wantOffset: 0,
		},
		{
			name:       "usa limite default quando ha offset",
			limit:      0,
			offset:     1,
			wantLimit:  database.DefaultConversationPageLimit,
			wantOffset: 1,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotLimit, gotOffset := normalizeConversationPageRequest(tt.limit, tt.offset)
			if gotLimit != tt.wantLimit || gotOffset != tt.wantOffset {
				t.Fatalf("normalizeConversationPageRequest(%d, %d) = (%d, %d), want (%d, %d)",
					tt.limit, tt.offset, gotLimit, gotOffset, tt.wantLimit, tt.wantOffset)
			}
		})
	}
}
