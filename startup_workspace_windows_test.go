package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestInstallerWorkspaceCommandsAcceptDriveRootAndSpaces(t *testing.T) {
	data, err := os.ReadFile("build/windows/installer/project.nsi")
	if err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(t.TempDir(), "pasta com espaços")
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	root := filepath.VolumeName(project) + `\`
	commands := 0
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.Contains(line, "WriteRegStr") || !strings.Contains(line, `OpenAssistente\command`) {
			continue
		}
		commands++
		start, end := strings.Index(line, "'"), strings.LastIndex(line, "'")
		if start < 0 || end <= start {
			t.Fatalf("invalid command: %s", line)
		}
		for _, folder := range []string{root, project} {
			command := strings.NewReplacer("$INSTDIR", `C:\Program Files\Assistente`, "${PRODUCT_EXECUTABLE}", "assistente.exe", "%V", folder).Replace(line[start+1 : end])
			args, err := windows.DecomposeCommandLine(command)
			if err != nil || len(args) != 2 {
				t.Fatalf("command %q: args %v, error %v", command, args, err)
			}
			got, _, err := parseWorkspaceArgs(args[1:])
			if err != nil || got != filepath.Clean(folder) {
				t.Fatalf("folder %q: got %q, error %v", folder, got, err)
			}
		}
	}
	if commands != 2 {
		t.Fatalf("tested %d context menu commands, want 2", commands)
	}
}
