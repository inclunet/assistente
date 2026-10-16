package commandconfig

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestStreamDeckTriggerPortNormalizeAndParseRoundTrip(t *testing.T) {
	port := StreamDeckTriggerPort{}
	identity, err := port.Normalize(context.Background(), []byte(`{"version":1,"device":"AL28K2C54852","key":7}`))
	if err != nil || identity != "streamdeck.key:AL28K2C54852:key:7" {
		t.Fatalf("normalize = %q, %v", identity, err)
	}
	spec, err := ParseStreamDeckTriggerIdentity(identity)
	if err != nil || spec != (StreamDeckTriggerSpec{Device: "AL28K2C54852", Key: 7}) {
		t.Fatalf("parse = %#v, %v", spec, err)
	}
	if err := port.ValidateIdentity(context.Background(), identity); err != nil {
		t.Fatalf("identidade canônica rejeitada: %v", err)
	}
}

func TestStreamDeckTriggerPortRejectsNonCanonicalDocumentsAndIdentities(t *testing.T) {
	port := StreamDeckTriggerPort{}
	for _, raw := range []string{
		`{"version":1,"device":"serial","key":0,"extra":true}`,
		`{"version":1,"device":"serial","key":0,"key":1}`,
		`{"version":1,"device":"serial","key":1.0}`,
		`{"version":1,"device":"serial","key":null}`,
		`{"version":1,"device":"serial","key":true}`,
		`{"version":1,"device":"serial","key":"1"}`,
		`{"version":1,"device":"serial","key":-1}`,
		`{"version":1,"device":"serial","key":256}`,
		`{"version":1,"device":"serial!","key":0}`,
		`{"version":1,"device":"","key":0}`,
		`{"version":2,"device":"serial","key":0}`,
	} {
		if _, err := port.Normalize(context.Background(), []byte(raw)); !errors.Is(err, ErrInvalid) {
			t.Fatalf("documento aceito %s: %v", raw, err)
		}
	}
	for _, identity := range []string{
		"streamdeck.key:serial:key:01", "streamdeck.key:serial:key:256",
		"streamdeck.key:serial!:key:0", "streamdeck.key:serial:key:0:extra",
		"keyboard.local:KeyK", "streamdeck.key::key:0",
	} {
		if _, err := ParseStreamDeckTriggerIdentity(identity); !errors.Is(err, ErrInvalid) {
			t.Fatalf("identidade aceita %q: %v", identity, err)
		}
	}
	long := strings.Repeat("a", 129)
	if _, err := port.Normalize(context.Background(), []byte(`{"version":1,"device":"`+long+`","key":0}`)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("serial longo aceito: %v", err)
	}
}

func TestStreamDeckTriggerPortCancellation(t *testing.T) {
	port := StreamDeckTriggerPort{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := port.Normalize(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("Normalize cancelado = %v", err)
	}
	if err := port.ValidateIdentity(ctx, "streamdeck.key:serial:key:0"); !errors.Is(err, context.Canceled) {
		t.Fatalf("ValidateIdentity cancelado = %v", err)
	}
}

func TestStreamDeckTriggerPortSatisfiesProjectionPorts(t *testing.T) {
	var _ TriggerPort = StreamDeckTriggerPort{}
	var _ TriggerIdentityValidator = StreamDeckTriggerPort{}
}
