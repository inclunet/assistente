package app

import (
	"context"
	"sort"

	"assistente/internal/commandconfig"
)

type commandDeckDiscoveredDevice struct {
	ID       string `json:"id"`
	Model    string `json:"model"`
	KeyCount int    `json:"keyCount"`
}

type commandDeckDiscovery struct {
	Status  string                        `json:"status"`
	Devices []commandDeckDiscoveredDevice `json:"devices"`
}

// Discovery is read-only: enumeration never opens or claims the device.
// This private projection supports capture readiness; settings no longer
// publish device identifiers or offer device selection.
func (p *commandProductRuntime) discoverDeck(ctx context.Context) commandDeckDiscovery {
	result := commandDeckDiscovery{Status: "unavailable", Devices: []commandDeckDiscoveredDevice{}}
	p.mu.Lock()
	driver := p.deckDriver
	p.mu.Unlock()
	if driver == nil {
		return result
	}
	devices, err := driver.Enumerate(ctx)
	if err != nil || ctx.Err() != nil {
		return result
	}
	seen := map[string]bool{}
	for _, device := range devices {
		id := string(device.ID)
		if _, err := commandconfig.ParseStreamDeckTriggerIdentity("streamdeck.key:" + id + ":key:0"); err != nil {
			continue
		}
		count := int(device.Model.KeyCount())
		if count < 1 || count > 256 || seen[id] {
			continue
		}
		seen[id] = true
		result.Devices = append(result.Devices, commandDeckDiscoveredDevice{ID: id, Model: device.Model.Name, KeyCount: count})
	}
	sort.Slice(result.Devices, func(i, j int) bool { return result.Devices[i].ID < result.Devices[j].ID })
	result.Status = "ready"
	return result
}
