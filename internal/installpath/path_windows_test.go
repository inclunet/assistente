package installpath

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"
)

func testRegistry(t *testing.T) (registry.Key, registry.Key, string) {
	t.Helper()
	path := fmt.Sprintf(`Software\Inclunet\AssistenteTests\InstallerPath-%d`, time.Now().UnixNano())
	env, _, err := registry.CreateKey(registry.CURRENT_USER, path+`\Environment`, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	owners, _, err := registry.CreateKey(registry.CURRENT_USER, path+`\Owners`, registry.ALL_ACCESS)
	if err != nil {
		_ = env.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = env.Close()
		_ = owners.Close()
		_ = registry.DeleteKey(registry.CURRENT_USER, path+`\Environment`)
		_ = registry.DeleteKey(registry.CURRENT_USER, path+`\Owners`)
		_ = registry.DeleteKey(registry.CURRENT_USER, path)
	})
	return env, owners, path
}

func TestRegistryPathLifecycle(t *testing.T) {
	for _, kind := range []uint32{registry.SZ, registry.EXPAND_SZ} {
		t.Run(fmt.Sprint(kind), func(t *testing.T) {
			env, owners, _ := testRegistry(t)
			const dir = `C:\Program Files\Inclunet\Assistente`
			const sid = "S-1-5-21-1-2-3-1001"
			const original = `%USERPROFILE%\bin;C:\Tools;`
			if err := writePath(env, original, kind); err != nil {
				t.Fatal(err)
			}
			for range 2 {
				if err := update(env, owners, sid, dir, false); err != nil {
					t.Fatal(err)
				}
			}
			value, gotKind, err := readPath(env)
			want, _ := addEntry(original, dir)
			if err != nil || value != want || gotKind != kind {
				t.Fatalf("PATH/type changed: %q %d %v", value, gotKind, err)
			}
			// Uma entrada acrescentada pelo usuário depois da instalação sobrevive.
			if err := writePath(env, value+`;C:\Later`, kind); err != nil {
				t.Fatal(err)
			}
			if err := update(env, owners, sid, dir, true); err != nil {
				t.Fatal(err)
			}
			value, gotKind, err = readPath(env)
			if err != nil || value != original+`;C:\Later` || gotKind != kind {
				t.Fatalf("uninstall changed other entries: %q %d %v", value, gotKind, err)
			}
			if _, _, err := owners.GetStringValue(sid); !errors.Is(err, registry.ErrNotExist) {
				t.Fatalf("ownership left behind: %v", err)
			}
		})
	}
}

func TestRegistryPreexistingPathIsPreservedOnUninstall(t *testing.T) {
	env, owners, _ := testRegistry(t)
	const dir = `C:\Program Files\Inclunet\Assistente`
	const sid = "S-1-5-21-1-2-3-1001"
	if err := env.SetStringValue("Path", dir); err != nil {
		t.Fatal(err)
	}
	if err := update(env, owners, sid, dir, false); err != nil {
		t.Fatal(err)
	}
	if err := update(env, owners, sid, dir, true); err != nil {
		t.Fatal(err)
	}
	if got, _, err := readPath(env); err != nil || got != dir {
		t.Fatalf("removed preexisting PATH: %q %v", got, err)
	}
}

func TestRegistryWriteFailureDoesNotClaimPath(t *testing.T) {
	_, owners, path := testRegistry(t)
	readOnly, err := registry.OpenKey(registry.CURRENT_USER, path+`\Environment`, registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = readOnly.Close() }()
	const sid = "S-1-5-21-1-2-3-1001"
	if err := update(readOnly, owners, sid, `C:\App`, false); err == nil {
		t.Fatal("write unexpectedly succeeded")
	}
	if _, _, err := owners.GetStringValue(sid); !errors.Is(err, registry.ErrNotExist) {
		t.Fatalf("failed write acquired ownership: %v", err)
	}
}

func TestUnavailableProfileDoesNotBlockOtherCleanup(t *testing.T) {
	env, owners, path := testRegistry(t)
	const offline = "S-1-5-21-1-2-3-1001"
	const online = "S-1-5-21-1-2-3-1002"
	const dir = `C:\Program Files\Inclunet\Assistente`
	for _, sid := range []string{offline, online} {
		if err := owners.SetStringValue(sid, dir); err != nil {
			t.Fatal(err)
		}
	}
	if err := env.SetStringValue("Path", dir+`;C:\Other`); err != nil {
		t.Fatal(err)
	}
	err := updateUsers(owners, []string{offline, online}, dir, true, func(sid string) (registry.Key, error) {
		if sid == offline {
			return 0, registry.ErrNotExist
		}
		return registry.OpenKey(registry.CURRENT_USER, path+`\Environment`, registry.QUERY_VALUE|registry.SET_VALUE)
	})
	var warning *CleanupWarning
	if !errors.As(err, &warning) {
		t.Fatalf("expected nonblocking cleanup warning: %v", err)
	}
	if got, _, err := readPath(env); err != nil || got != `C:\Other` {
		t.Fatalf("available user was not cleaned: %q %v", got, err)
	}
	if _, _, err := owners.GetStringValue(offline); err != nil {
		t.Fatalf("lost pending cleanup marker: %v", err)
	}
	if _, _, err := owners.GetStringValue(online); !errors.Is(err, registry.ErrNotExist) {
		t.Fatalf("online marker survived: %v", err)
	}
}

func TestStationUserSIDRejectsMissingOrMalformedIdentity(t *testing.T) {
	valid := []byte{1, 1, 0, 0, 0, 0, 0, 5, 21, 0, 0, 0}
	if got, err := stationUserSID(valid, uint32(len(valid))); err != nil || got != "S-1-5-21" {
		t.Fatalf("valid station SID: %q %v", got, err)
	}
	for _, value := range []struct {
		buffer []byte
		length uint32
	}{
		{nil, 0}, {valid, 0}, {valid, 100}, {valid[:8], 8},
		{[]byte{2, 0, 0, 0, 0, 0, 0, 5}, 8},
	} {
		if _, err := stationUserSID(value.buffer, value.length); err == nil {
			t.Fatal("accepted missing or malformed station user")
		}
	}
}

// Somente leitura; opt-in para uma sessão real, sem tocar no Registro/PATH.
func TestInteractiveUserSIDReadOnly(t *testing.T) {
	if os.Getenv("ASSISTENTE_TEST_INTERACTIVE_SID") != "1" {
		t.Skip("requer sessão interativa")
	}
	sid, err := InteractiveUserSID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := windows.StringToSid(sid); err != nil {
		t.Fatal(err)
	}
}
