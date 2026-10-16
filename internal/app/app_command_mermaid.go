package app

func mermaidSubmitShortcut(shortcut LocalCommandShortcut) bool {
	if shortcut.Version != 1 || shortcut.Steps != nil || len(shortcut.Modifiers) != 1 {
		return false
	}
	return shortcut.Code == "KeyS" && (shortcut.Modifiers[0] == "Control" || shortcut.Modifiers[0] == "Meta") || shortcut.Code == "Enter" && shortcut.Modifiers[0] == "Control"
}

func isMermaidMutation(id string) bool {
	return id == commandEditorMermaidApplyID || id == commandEditorMermaidRemoveID
}
