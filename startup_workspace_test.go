package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseWorkspaceArgs(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	folder := filepath.Join(root, "pasta com espaços")
	if err := os.Mkdir(folder, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "file"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name    string
		args    []string
		want    string
		runtime []string
		invalid bool
	}{
		{name: "restore"},
		{name: "current", args: []string{"."}, want: root},
		{name: "absolute", args: []string{folder}, want: folder},
		{name: "relative", args: []string{"pasta com espaços"}, want: folder},
		{name: "terminator", args: []string{"--", "."}, want: root},
		{name: "dev", args: []string{"-assetdir", "frontend/dist", ".", "-loglevel=Debug"}, want: root, runtime: []string{"-assetdir", "frontend/dist", "-loglevel=Debug"}},
		{name: "bindings", args: []string{"-tsprefix", "", "-tssuffix", "DTO", "-tsoutputtype", "interfaces"}, runtime: []string{"-tsprefix", "", "-tssuffix", "DTO", "-tsoutputtype", "interfaces"}},
		{name: "multiple", args: []string{".", folder}, invalid: true},
		{name: "removed", args: []string{"--workdir", root}, invalid: true},
		{name: "removedEquals", args: []string{"--workdir=" + root}, invalid: true},
		{name: "unknown", args: []string{"--typo"}, invalid: true},
		{name: "missing", args: []string{"missing"}, invalid: true},
		{name: "file", args: []string{"file"}, invalid: true},
		{name: "empty", args: []string{""}, invalid: true},
		{name: "missingRuntimeValue", args: []string{"-assetdir"}, invalid: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir, args, err := parseWorkspaceArgs(tt.args)
			if (err != nil) != tt.invalid {
				t.Fatalf("error = %v", err)
			}
			if tt.invalid {
				return
			}
			if dir != tt.want || !reflect.DeepEqual(args, tt.runtime) {
				t.Fatalf("got (%q, %v), want (%q, %v)", dir, args, tt.want, tt.runtime)
			}
			if cwd, _ := os.Getwd(); cwd != root {
				t.Fatalf("changed cwd: %q", cwd)
			}
		})
	}
}
