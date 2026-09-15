package commandcontext

import (
	"reflect"
	"testing"

	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"github.com/google/uuid"
)

func cacheScope(t *testing.T) Scope {
	t.Helper()
	user, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	ws := "ws-0192f3a4b5c6d7e8"
	return Scope{UserID: user.String(), AuthContextID: "cache-auth", WorkspaceID: &ws}
}

func cacheFixtureKey(t *testing.T) CacheKey {
	t.Helper()
	scope := cacheScope(t)
	workspaceGeneration := "startup:workspace:1"
	return CacheKey{
		UserID: scope.UserID, WorkspaceID: scope.WorkspaceID, TriggerIdentity: "keyboard.local:Ctrl+K",
		SourceType: commandcatalog.KeyboardLocal, ContextVersion: "context-v1", RegistryVersion: "registry-v1",
		GlobalConfigGeneration: "startup:global:1", WorkspaceConfigGeneration: &workspaceGeneration,
		ActiveLayersGeneration: "layers-v1",
	}
}

func TestResolutionCacheChaveiaTodasDimensoesENilWorkspaceTipado(t *testing.T) {
	cache, err := NewResolutionCache(16)
	if err != nil {
		t.Fatal(err)
	}
	base := cacheFixtureKey(t)
	result := commandbindings.Result{Status: commandbindings.Selected, CommandID: "workspace.tab.new", BindingIDs: []string{"binding"}}
	if err := cache.Put(base, result); err != nil {
		t.Fatal(err)
	}
	result.BindingIDs[0] = "mutated-input"
	got, ok := cache.Get(base)
	if !ok || !reflect.DeepEqual(got.BindingIDs, []string{"binding"}) {
		t.Fatalf("resultado não detached: %#v %v", got, ok)
	}
	got.BindingIDs[0] = "mutated-output"
	again, _ := cache.Get(base)
	if again.BindingIDs[0] != "binding" {
		t.Fatal("cache entregou slice interno")
	}

	variants := []func(*CacheKey){
		func(k *CacheKey) { k.UserID = "0192f3a4-b5c6-7de8-8000-000000000001" },
		func(k *CacheKey) { k.WorkspaceID = nil; k.WorkspaceConfigGeneration = nil },
		func(k *CacheKey) { v := "ws-0192f3a4b5c6d7e9"; k.WorkspaceID = &v },
		func(k *CacheKey) { k.TriggerIdentity = "keyboard.local:Ctrl+L" },
		func(k *CacheKey) { k.SourceType = commandcatalog.UI },
		func(k *CacheKey) { k.ContextVersion = "context-v2" },
		func(k *CacheKey) { k.RegistryVersion = "registry-v2" },
		func(k *CacheKey) { k.GlobalConfigGeneration = "startup:global:2" },
		func(k *CacheKey) { v := "startup:workspace:2"; k.WorkspaceConfigGeneration = &v },
		func(k *CacheKey) { k.ActiveLayersGeneration = "layers-v2" },
	}
	for index, change := range variants {
		t.Run(string(rune('a'+index)), func(t *testing.T) {
			variant := base
			change(&variant)
			if _, ok := cache.Get(variant); ok {
				t.Fatal("dimensão alterada reutilizou resultado")
			}
		})
	}

	globalKey := base
	globalKey.WorkspaceID = nil
	globalKey.WorkspaceConfigGeneration = nil
	if err := cache.Put(globalKey, commandbindings.Result{Status: commandbindings.NoMatch}); err != nil {
		t.Fatal(err)
	}
	if _, ok := cache.Get(globalKey); !ok {
		t.Fatal("workspace nil tipado não foi preservado")
	}
}

func TestResolutionCacheBoundedResultadosNegativosELRU(t *testing.T) {
	cache, err := NewResolutionCache(2)
	if err != nil {
		t.Fatal(err)
	}
	first, second, third := cacheFixtureKey(t), cacheFixtureKey(t), cacheFixtureKey(t)
	second.TriggerIdentity, third.TriggerIdentity = "keyboard.local:Ctrl+L", "keyboard.local:Ctrl+M"
	if err := cache.Put(first, commandbindings.Result{Status: commandbindings.NoMatch}); err != nil {
		t.Fatal(err)
	}
	if err := cache.Put(second, commandbindings.Result{Status: commandbindings.Conflict, BindingIDs: []string{"a", "b"}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := cache.Get(first); !ok {
		t.Fatal("resultado negativo ausente")
	}
	if err := cache.Put(third, commandbindings.Result{Status: commandbindings.Blocked}); err != nil {
		t.Fatal(err)
	}
	if cache.Len() != 2 {
		t.Fatalf("cache excedeu limite: %d", cache.Len())
	}
	if _, ok := cache.Get(second); ok {
		t.Fatal("LRU não removeu o item antigo")
	}
	if _, ok := cache.Get(first); !ok {
		t.Fatal("LRU removeu item recentemente usado")
	}
}
