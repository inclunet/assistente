package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"math/rand"
	"strings"
	"testing"

	"assistente/internal/commandconfig"
	"assistente/internal/commanddecision"
	"assistente/internal/commanddeck"
	"assistente/internal/commandimage"
	"assistente/internal/database"
	"assistente/internal/questionnaire"
	"gorm.io/gorm"
)

func settingsImageUpload(t *testing.T) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 256, 256))
	rng := rand.New(rand.NewSource(7))
	_, _ = rng.Read(img.Pix)
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	if data.Len() <= 64<<10 {
		t.Fatal("fixture must exceed document limit")
	}
	return base64.StdEncoding.EncodeToString(data.Bytes())
}

func TestCommandSettingsImageTransientUploadIsNotConfiguration(t *testing.T) {
	upload := settingsImageUpload(t)
	original := CommandSettingsMutationRequest{Operation: "binding_create", Binding: &CommandSettingsBindingInput{TriggerType: "streamdeck.key", Presentation: map[string]any{"version": 1, "image_upload": upload, "icon": "star"}}}
	normalized, asset, err := prepareCommandSettingsImage(original)
	if err != nil || asset == nil {
		t.Fatalf("normalize: %v", err)
	}
	if original.Binding.Presentation["image_upload"] != upload {
		t.Fatal("mutated caller")
	}
	if _, found := normalized.Binding.Presentation["image_upload"]; found {
		t.Fatal("upload leaked")
	}
	if normalized.Binding.Presentation["image_ref"] != asset.Ref || normalized.Binding.Presentation["icon"] != "star" {
		t.Fatal("missing metadata")
	}
	doc, _ := json.Marshal(normalized.Binding.Presentation)
	if len(doc) > 256 || strings.Contains(string(doc), upload) {
		t.Fatal("bytes in document")
	}
	for _, value := range []any{nil, 3, "", "file:///private", strings.Repeat("a", 2<<20)} {
		original.Binding.Presentation["image_upload"] = value
		if _, _, err := prepareCommandSettingsImage(original); err == nil {
			t.Fatalf("accepted %T", value)
		}
	}
}

func TestCommandSettingsImageDiffDetectsReplacementWithoutReadingDigest(t *testing.T) {
	beforeRef, afterRef := strings.Repeat("a", 64), strings.Repeat("b", 64)
	before := commandconfig.Binding{ID: "binding", Presentation: `{"version":1,"image_ref":"` + beforeRef + `"}`}
	after := before
	after.Presentation = `{"version":1,"image_ref":"` + afterRef + `"}`
	for locale, label := range map[string]string{"pt-BR": "Imagem personalizada", "en": "Custom image", "es": "Imagen personalizada"} {
		text, err := renderCommandSettingsDiff(locale, commandconfig.MutationDiff{Scope: commandconfig.Scope{UserID: "owner"}, BeforeBindings: []commandconfig.Binding{before}, AfterBindings: []commandconfig.Binding{after}})
		if err != nil || !strings.Contains(text, label) || strings.Contains(text, beforeRef) || strings.Contains(text, afterRef) {
			t.Fatalf("diff %s: %s %v", locale, text, err)
		}
	}
}

func TestCommandSettingsImageConfirmedRoundTripRemovalAndIsolation(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layer, _ := settingsActivationSecurityLayerAndRule(t, a, decisions)
	input := CommandSettingsBindingInput{LayerID: layer, CommandID: "navigation.settings.open", TriggerType: "streamdeck.key", TriggerSpec: `{"version":1,"device":"test-deck","key":0}`, Effect: "execute", Enabled: true, Presentation: map[string]any{"version": 1, "icon": "star", "image_upload": settingsImageUpload(t)}}
	created := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_create", Binding: &input})
	input.ID = created.ID
	if _, err := a.SetCommandLayerActive(layer, true); err != nil {
		t.Fatal(err)
	}
	if err := a.rebuildCommandLifecyclePersistedConfiguration(a.ctx); err != nil {
		t.Fatal(err)
	}
	p := a.commandProduct.Load()
	bindings, _, err := p.deckMap(a.ctx)
	if err != nil {
		t.Fatal(err)
	}
	binding := bindings["test-deck"][0]
	if !commandimage.ValidRef(binding.imageRef) {
		t.Fatal("missing projected image")
	}
	if _, err := commandimage.Load(a.ctx, database.DB(), "another-user", binding.imageRef); err == nil {
		t.Fatal("cross-owner asset")
	}
	model := commanddeck.Model{ID: "test", Name: "Test", Rows: 1, Columns: 1, KeyImageW: 72, KeyImageH: 72}
	view, retry := p.commandDeckImageKeyView(a.ctx, binding, "en", model)
	if retry {
		t.Fatal("healthy asset scheduled retry")
	}
	fallback := commandDeckKeyView(binding, "en", model)
	if bytes.Equal(view.ImageRGBA, fallback.ImageRGBA) || view.Title != fallback.Title || view.Announce != fallback.Announce {
		t.Fatal("image not rendered or lost text")
	}
	// A recoverable storage failure requests a background retry. The same
	// binding subsequently recovers without editing or reconnecting the device.
	db := database.DB()
	callback := "test:image_load_failure"
	if err := db.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "command_image_assets" {
			tx.AddError(errors.New("temporary image storage failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Query().Remove(callback) })
	failed, retry := p.commandDeckImageKeyView(a.ctx, binding, "en", model)
	if !retry || !bytes.Equal(failed.ImageRGBA, fallback.ImageRGBA) {
		t.Fatal("transient error did not schedule safe fallback retry")
	}
	if err := db.Callback().Query().Remove(callback); err != nil {
		t.Fatal(err)
	}
	recovered, retry := p.commandDeckImageKeyView(a.ctx, binding, "en", model)
	if retry || !bytes.Equal(recovered.ImageRGBA, view.ImageRGBA) {
		t.Fatal("image did not recover")
	}
	snapshot, err := a.GetCommandSettingsForScope("en", "global")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range snapshot.Bindings {
		if row.ID == input.ID {
			if _, ok := row.Presentation["image_upload"]; ok {
				t.Fatal("stored upload")
			}
			input.Presentation = row.Presentation
		}
	}
	// An unrelated confirmed edit remains below the unchanged document limit.
	input.Presentation["icon"] = "folder"
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_update", ID: input.ID, Binding: &input})
	delete(input.Presentation, "image_ref")
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_update", ID: input.ID, Binding: &input})
	if _, err := commandimage.Load(a.ctx, database.DB(), p.principal.UserID, binding.imageRef); err == nil {
		t.Fatal("orphan after removal")
	}
	missing, retry := p.commandDeckImageKeyView(a.ctx, binding, "en", model)
	if retry || !bytes.Equal(missing.ImageRGBA, fallback.ImageRGBA) {
		t.Fatal("missing image must use stable fallback")
	}
	if err := a.rebuildCommandLifecyclePersistedConfiguration(a.ctx); err != nil {
		t.Fatal(err)
	}
	bindings, _, err = p.deckMap(a.ctx)
	if err != nil || bindings["test-deck"][0].imageRef != "" {
		t.Fatalf("stale frame: %v", err)
	}
}

func TestCommandSettingsImageCancellationDoesNotPersistAsset(t *testing.T) {
	for _, mode := range []string{"denied", "session_revoked", "audit_failure"} {
		t.Run(mode, func(t *testing.T) { settingsImageRefused(t, mode) })
	}
}

func settingsImageRefused(t *testing.T, mode string) {
	t.Helper()
	a, decisions := settingsSecurityFixture(t)
	layer, _ := settingsActivationSecurityLayerAndRule(t, a, decisions)
	snapshot, err := a.GetCommandSettingsForScope("en", "global")
	if err != nil {
		t.Fatal(err)
	}
	if mode == "audit_failure" {
		if err := database.DB().Exec(`CREATE TRIGGER reject_image_audit BEFORE INSERT ON command_config_mutations BEGIN SELECT RAISE(ABORT, 'test audit failure'); END`).Error; err != nil {
			t.Fatal(err)
		}
	}
	req := CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_create", ExpectedRevision: snapshot.Revision, ExpectedFingerprint: snapshot.Fingerprint, Binding: &CommandSettingsBindingInput{LayerID: layer, CommandID: "navigation.settings.open", TriggerType: "streamdeck.key", TriggerSpec: `{"version":1,"device":"test-deck","key":0}`, Effect: "execute", Enabled: true, Presentation: map[string]any{"version": 1, "image_upload": settingsImageUpload(t)}}}
	done := settingsSecurityStart(t, a, func() (CommandSettingsMutation, error) { return a.MutateCommandSettings(req) })
	select {
	case payload := <-decisions:
		action := commanddecision.DenyAction
		if mode == "session_revoked" {
			if err := database.DB().Exec("UPDATE sessions SET revoked_at = CURRENT_TIMESTAMP WHERE id = ?", a.commandProduct.Load().principal.SessionID).Error; err != nil {
				t.Fatal(err)
			}
			action = commanddecision.ApplyAction
		}
		if mode == "audit_failure" {
			action = commanddecision.ApplyAction
		}
		finishCommandDecision(t, a.questionnaireMgr, payload, map[string]any{questionnaire.AnswerActionID: action}, false)
	case early := <-done:
		t.Fatalf("no decision: %+v", early)
	case <-a.ctx.Done():
		t.Fatal("timeout")
	}
	out := settingsSecurityFinish(t, done)
	if out.result.Committed {
		t.Fatal("cancel committed")
	}
	var count int64
	if err := database.DB().WithContext(context.Background()).Table("command_image_assets").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("asset leaked: %d %v", count, err)
	}
	if err := database.DB().Table("command_bindings").Where("trigger_type = ?", "streamdeck.key").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("binding leaked: %d %v", count, err)
	}
}
