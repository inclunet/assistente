package app

import "testing"

func TestCommandEditorFileResultSurvivesExecutionDeadline(t *testing.T) {
	for _, id := range []string{commandEditorFileOpenID, commandEditorFileSaveID, commandEditorFileSaveCopyID} {
		if commandUIResultTTL(id) <= commandUIRunTimeout(id) {
			t.Fatalf("%s: resultado expira antes de poder consultar o desfecho", id)
		}
	}
}
