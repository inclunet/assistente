package app

import (
	"errors"
	"testing"

	"assistente/internal/commanddeck"
)

func TestCommandDeckDiscoveryStatusPreservesCapabilitiesAndRedactsErrors(t *testing.T) {
	model := commanddeck.Model{Name: "Stream Deck", Rows: 3, Columns: 5}
	for _, tc := range []struct {
		name           string
		err            error
		snapshot       commanddeck.DeviceStatus
		snapshotErr    error
		status, reason string
		connected      bool
	}{
		{name: "aberto", snapshot: commanddeck.DeviceConnected, status: "connected", connected: true},
		{name: "handle próprio já aberto", err: commanddeck.ErrDeviceAlreadyOpen, snapshot: commanddeck.DeviceConnected, status: "connected", connected: true},
		{name: "falha de abertura", err: errors.New("access denied C:/private/serial"), snapshotErr: commanddeck.ErrInvalidDevice, status: "unavailable", reason: "open_failed"},
		{name: "sem snapshot não é conectado", err: commanddeck.ErrDeviceAlreadyOpen, snapshotErr: commanddeck.ErrInvalidDevice, status: "unavailable", reason: "open_failed"},
		{name: "aguardando reconexão", err: commanddeck.ErrReconnectBackoff, snapshot: commanddeck.DeviceDisconnected, status: "reconnecting", reason: "reconnect_backoff"},
		{name: "falha não mascara snapshot antigo", err: errors.New("open failed"), snapshot: commanddeck.DeviceConnected, status: "unavailable", reason: "open_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			device, connected := commandDeckDiscoveryStatus(
				commanddeck.DiscoverResult{Device: "test-device", Model: model, Err: tc.err},
				commanddeck.DeviceSnapshot{Model: model, Status: tc.snapshot}, tc.snapshotErr)
			if connected != tc.connected || device.Status != tc.status || device.Reason != tc.reason || device.Model != model.Name || device.KeyCount != 15 {
				t.Fatalf("diagnóstico incorreto: %+v connected=%v", device, connected)
			}
		})
	}
}
