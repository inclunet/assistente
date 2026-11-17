package installpath

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
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
	if strings.HasPrefix(sid, "S-1-5-5-") {
		t.Fatal("returned logon SID instead of account SID")
	}
	key, err := registry.OpenKey(registry.USERS, sid+`\Environment`, registry.QUERY_VALUE)
	if err != nil {
		t.Fatalf("account environment unavailable: %v", err)
	}
	_ = key.Close()
}

func TestAccountSIDForStation(t *testing.T) {
	user, err := windows.StringToSid("S-1-5-21-1-2-3-1001")
	if err != nil {
		t.Fatal(err)
	}
	logon, err := windows.StringToSid("S-1-5-5-100-200")
	if err != nil {
		t.Fatal(err)
	}
	group := windows.SIDAndAttributes{Sid: logon, Attributes: windows.SE_GROUP_LOGON_ID}
	for _, tc := range []struct {
		name    string
		user    *windows.SID
		groups  []windows.SIDAndAttributes
		station string
		want    bool
	}{
		{"account of matching logon", user, []windows.SIDAndAttributes{group}, logon.String(), true},
		{"missing account", nil, []windows.SIDAndAttributes{group}, logon.String(), false},
		{"logon is not account", logon, []windows.SIDAndAttributes{group}, logon.String(), false},
		{"missing groups", user, nil, logon.String(), false},
		{"different logon", user, []windows.SIDAndAttributes{group}, "S-1-5-5-100-201", false},
		{"ordinary group", user, []windows.SIDAndAttributes{{Sid: logon}}, logon.String(), false},
		{"missing group SID", user, []windows.SIDAndAttributes{{Attributes: windows.SE_GROUP_LOGON_ID}}, logon.String(), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := accountSIDForStation(tc.user, tc.groups, tc.station)
			if tc.want {
				if err != nil || got != user.String() {
					t.Fatalf("account = %q, error = %v", got, err)
				}
			} else if err == nil || got != "" {
				t.Fatalf("accepted invalid identity: %q, %v", got, err)
			}
		})
	}
}

func TestTokenPrivilegeRestoredAfterQuery(t *testing.T) {
	var original, token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_DUPLICATE|windows.TOKEN_QUERY, &original); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = original.Close() }()
	if err := windows.DuplicateTokenEx(original, windows.TOKEN_QUERY|windows.TOKEN_ADJUST_PRIVILEGES, nil, windows.SecurityImpersonation, windows.TokenImpersonation, &token); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = token.Close() }()
	// Modifica somente uma cópia não aplicada a nenhum processo/thread.
	state := windows.Tokenprivileges{PrivilegeCount: 1}
	name, _ := windows.UTF16PtrFromString("SeChangeNotifyPrivilege")
	if err := windows.LookupPrivilegeValue(nil, name, &state.Privileges[0].Luid); err != nil {
		t.Fatal(err)
	}
	if err := windows.AdjustTokenPrivileges(token, false, &state, 0, nil, nil); err != nil {
		t.Fatal(err)
	}
	read := func() []byte {
		t.Helper()
		var needed uint32
		_ = windows.GetTokenInformation(token, windows.TokenPrivileges, nil, 0, &needed)
		buffer := make([]byte, needed)
		if err := windows.GetTokenInformation(token, windows.TokenPrivileges, &buffer[0], needed, &needed); err != nil {
			t.Fatal(err)
		}
		return buffer
	}
	before := read()
	for _, failure := range []error{nil, errors.New("query denied")} {
		called := false
		err := withTokenPrivilege(token, "SeChangeNotifyPrivilege", func() error {
			called = true
			buffer := read()
			actual := (*windows.Tokenprivileges)(unsafe.Pointer(&buffer[0]))
			for _, entry := range actual.AllPrivileges() {
				if entry.Luid == state.Privileges[0].Luid && entry.Attributes&windows.SE_PRIVILEGE_ENABLED != 0 {
					return failure
				}
			}
			t.Error("query ran without enabled privilege")
			return failure
		})
		if !called || !errors.Is(err, failure) {
			t.Fatalf("query result: called=%v error=%v", called, err)
		}
		if !bytes.Equal(before, read()) {
			t.Fatal("privilege state not restored")
		}
	}
	// Privilégio removido da cópia não pode ser concedido pela função.
	state.Privileges[0].Attributes = windows.SE_PRIVILEGE_REMOVED
	if err := windows.AdjustTokenPrivileges(token, false, &state, 0, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := withTokenPrivilege(token, "SeChangeNotifyPrivilege", func() error { t.Error("query ran with unavailable privilege"); return nil }); !errors.Is(err, windows.ERROR_NOT_ALL_ASSIGNED) {
		t.Fatalf("missing privilege: %v", err)
	}
}
