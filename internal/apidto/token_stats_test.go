package apidto

import "testing"

func TestTokenStatsJSONTagsStable(t *testing.T) {
	t.Parallel()
	var s TokenStats
	s.ConversationID = "c1"
	s.ToolBreakdown = []ToolUsageBreakdown{{ToolName: "x"}}
	if s.ConversationID != "c1" || len(s.ToolBreakdown) != 1 {
		t.Fatal("DTO tokens deve permanecer utilizável após extração para apidto")
	}
}
