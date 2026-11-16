package main

import "testing"

func TestInstallerPathCommandDoesNotInitializeApp(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"_installer-path"})
	if err != nil || cmd == rootCmd || !cmd.Hidden {
		t.Fatalf("internal command not found: %v", err)
	}
	if cmd.Annotations["skipInit"] != "true" {
		t.Fatal("installer would initialize the app")
	}
	if err := rootCmd.PersistentPreRunE(cmd, []string{"add"}); err != nil {
		t.Fatal(err)
	}
	if rootApp != nil {
		t.Fatal("installer initialized app services")
	}
	for _, args := range [][]string{nil, {"unknown"}, {"add", "extra"}} {
		if err := cmd.ValidateArgs(args); err == nil {
			t.Fatalf("accepted invalid args: %v", args)
		}
	}
}
