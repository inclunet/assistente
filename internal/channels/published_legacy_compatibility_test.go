package channels

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"assistente/internal/configdir"
	"assistente/internal/database"
)

func TestPublished019ChannelsImportDirectlyIdempotentlyAndPreserveSources(t *testing.T) {
	setupTempHome(t)
	db := setupChannelsDB(t)

	fixtureRoot := filepath.Join("testdata", "published", "0.1.9")
	originals := copyPublishedChannelFixture(t, fixtureRoot, configdir.GetHomeDir())

	ctx := database.WithUserID(context.Background(), "published-fixture-user")
	first, err := ImportLegacyChannelsWithContext(ctx, nil)
	if err != nil {
		t.Fatalf("importação direta da 0.1.9: %v", err)
	}
	if first.Imported != 2 || first.Failed != 1 {
		t.Fatalf("resultado inesperado: %+v", first)
	}

	cfg, err := Load("telegram")
	if err != nil {
		t.Fatalf("carregar canal importado: %v", err)
	}
	if cfg == nil || cfg.Conversations["contact-fixture"] != "7" ||
		cfg.BotTokenRef != "channel:telegram:bot_token" {
		t.Fatalf("dados do canal não foram preservados: %+v", cfg)
	}
	var contacts int64
	if err := db.Model(&database.ChannelContact{}).
		Where("external_id = ? AND display_name = ?", "contact-fixture", "Pessoa Sintética").
		Count(&contacts).Error; err != nil {
		t.Fatalf("consultar contato: %v", err)
	}
	if contacts != 1 {
		t.Fatalf("contato publicado não foi preservado: count=%d", contacts)
	}

	second, err := ImportLegacyChannelsWithContext(ctx, nil)
	if err != nil {
		t.Fatalf("segunda importação: %v", err)
	}
	if second.Imported != 0 || second.Skipped != 2 || second.Failed != 1 {
		t.Fatalf("reimportação não foi idempotente: %+v", second)
	}
	assertPublishedChannelSourcesUnchanged(t, configdir.GetHomeDir(), originals)
}

func copyPublishedChannelFixture(t *testing.T, sourceRoot, destinationRoot string) map[string][]byte {
	t.Helper()
	originals := make(map[string][]byte)
	err := filepath.Walk(sourceRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(sourceRoot, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		destination := filepath.Join(destinationRoot, relative)
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(destination, data, 0o600); err != nil {
			return err
		}
		originals[relative] = data
		return nil
	})
	if err != nil {
		t.Fatalf("copiar corpus publicado: %v", err)
	}
	return originals
}

func assertPublishedChannelSourcesUnchanged(t *testing.T, root string, originals map[string][]byte) {
	t.Helper()
	for relative, want := range originals {
		got, err := os.ReadFile(filepath.Join(root, relative))
		if err != nil {
			t.Fatalf("fonte %s deixou de existir: %v", relative, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("fonte %s foi alterada", relative)
		}
	}
}
