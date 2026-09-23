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
	"assistente/internal/commandexecution"
	"assistente/internal/commandimage"
	"assistente/internal/database"
	"assistente/internal/questionnaire"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func settingsImageUpload(t *testing.T) string {
	return settingsImageUploadSeed(t, 7)
}

func settingsImageUploadSeed(t *testing.T, seed int64) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 256, 256))
	rng := rand.New(rand.NewSource(seed))
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
	normalized, batch, err := prepareCommandSettingsImage(original)
	if err != nil || batch == nil || len(batch.assets) != 1 {
		t.Fatalf("normalize: %v", err)
	}
	asset := batch.assets[0]
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

func TestCommandSettingsImageBatchNormalizesBaseAndVariantsWithoutMutatingRequest(t *testing.T) {
	baseUpload := settingsImageUpload(t)
	stateUpload := settingsImageUploadSeed(t, 19)
	input := CommandSettingsMutationRequest{Operation: "binding_update", Binding: &CommandSettingsBindingInput{Presentation: map[string]any{
		"icon": "star", "image_upload": baseUpload,
		"states": map[string]any{
			"on":  map[string]any{"title_by_locale": map[string]any{"en": "On"}, "image_upload": stateUpload},
			"off": map[string]any{"icon": "circle"},
		},
	}}}
	prepared, batch, err := prepareCommandSettingsImage(input)
	if err != nil || batch == nil || len(batch.assets) != 2 {
		t.Fatalf("prepare mixed batch: assets=%v err=%v", batch, err)
	}
	baseRef, _ := prepared.Binding.Presentation["image_ref"].(string)
	states := prepared.Binding.Presentation["states"].(map[string]any)
	on := states["on"].(map[string]any)
	stateRef, _ := on["image_ref"].(string)
	if !commandimage.ValidRef(baseRef) || !commandimage.ValidRef(stateRef) || baseRef == stateRef {
		t.Fatalf("references not normalized distinctly: %q %q", baseRef, stateRef)
	}
	if _, found := on["image_upload"]; found {
		t.Fatal("variant upload leaked")
	}
	if input.Binding.Presentation["image_upload"] != baseUpload || input.Binding.Presentation["states"].(map[string]any)["on"].(map[string]any)["image_upload"] != stateUpload {
		t.Fatal("mutated caller's nested presentation")
	}

	bad := CommandSettingsMutationRequest{Operation: "binding_create", Binding: &CommandSettingsBindingInput{Presentation: map[string]any{
		"states": map[string]any{
			"on":  map[string]any{"image_upload": stateUpload},
			"off": map[string]any{"image_upload": "invalid-late-upload"},
		},
	}}}
	if _, _, err := prepareCommandSettingsImage(bad); err == nil {
		t.Fatal("accepted invalid late upload")
	}
	if bad.Binding.Presentation["states"].(map[string]any)["on"].(map[string]any)["image_upload"] != stateUpload {
		t.Fatal("failed batch mutated caller's request")
	}
	conflict := CommandSettingsMutationRequest{Operation: "binding_update", Binding: &CommandSettingsBindingInput{Presentation: map[string]any{
		"states": map[string]any{"running": map[string]any{"image_ref": strings.Repeat("e", 64), "image_upload": stateUpload}},
	}}}
	if _, _, err := prepareCommandSettingsImage(conflict); err == nil {
		t.Fatal("accepted upload and existing reference in the same variant")
	}
}

func TestCommandSettingsImageCommitRollsBackBatchWhenVariantReferenceIsNotOwned(t *testing.T) {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := commandimage.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE command_bindings (user_id TEXT NOT NULL, presentation TEXT NOT NULL)`).Error; err != nil {
		t.Fatal(err)
	}
	upload, err := base64.StdEncoding.DecodeString(settingsImageUpload(t))
	if err != nil {
		t.Fatal(err)
	}
	asset, err := commandimage.Normalize(upload)
	if err != nil {
		t.Fatal(err)
	}
	missingVariantRef := strings.Repeat("f", 64)
	if asset.Ref == missingVariantRef {
		t.Fatal("unexpected fixture digest collision")
	}
	intent := commandconfig.MutationIntent{Binding: &commandconfig.Binding{Presentation: `{"image_ref":"` + asset.Ref + `","states":{"on":{"image_ref":"` + missingVariantRef + `"}}}`}}
	batch := &commandSettingsImageBatch{assets: []commandimage.Asset{asset}}
	err = db.Transaction(func(tx *gorm.DB) error {
		return commitCommandSettingsImage(ctx, tx, "owner-a", intent, batch, nil)
	})
	if !errors.Is(err, commandexecution.ErrInvalidRequest) {
		t.Fatalf("expected missing/non-owned variant ref rejection, got %v", err)
	}
	var count int64
	if err := db.Table("command_image_assets").Where("user_id = ?", "owner-a").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("partial asset batch persisted after rollback: %d", count)
	}
}

func TestCommandSettingsImageDiffDetectsReplacementWithoutReadingDigest(t *testing.T) {
	beforeRef, afterRef := strings.Repeat("a", 64), strings.Repeat("b", 64)
	beforeStateRef, afterStateRef := strings.Repeat("c", 64), strings.Repeat("d", 64)
	before := commandconfig.Binding{ID: "binding", Presentation: `{"version":1,"image_ref":"` + beforeRef + `","states":{"running":{"image_ref":"` + beforeStateRef + `"}}}`}
	after := before
	after.Presentation = `{"version":1,"image_ref":"` + afterRef + `","states":{"running":{"image_ref":"` + afterStateRef + `"}}}`
	canonicalPresentation := after.Presentation
	for locale, label := range map[string]string{"pt-BR": "Imagem personalizada", "en": "Custom image", "es": "Imagen personalizada"} {
		text, err := renderCommandSettingsDiff(locale, commandconfig.MutationDiff{Scope: commandconfig.Scope{UserID: "owner"}, BeforeBindings: []commandconfig.Binding{before}, AfterBindings: []commandconfig.Binding{after}})
		if err != nil || strings.Count(text, label) < 2 || strings.Contains(text, beforeRef) || strings.Contains(text, afterRef) || strings.Contains(text, beforeStateRef) || strings.Contains(text, afterStateRef) {
			t.Fatalf("diff %s: %s %v", locale, text, err)
		}
	}
	if after.Presentation != canonicalPresentation {
		t.Fatal("confirmation redaction mutated canonical presentation")
	}
}

func TestCommandSettingsImageConfirmedRoundTripRemovalAndIsolation(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layer, _ := settingsActivationSecurityLayerAndRule(t, a, decisions)
	input := CommandSettingsBindingInput{LayerID: layer, CommandID: "navigation.settings.open", TriggerType: "streamdeck.key", TriggerSpec: `{"version":1,"device":"test-deck","key":0}`, Effect: "execute", Enabled: true, Presentation: map[string]any{
		"version": 1, "icon": "star", "image_upload": settingsImageUpload(t),
		"states": map[string]any{"running": map[string]any{"image_upload": settingsImageUploadSeed(t, 29)}},
	}}
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
	runningVisual, ok := binding.variants["running"]
	if !ok || !commandimage.ValidRef(runningVisual.imageRef) || runningVisual.imageRef == binding.imageRef {
		t.Fatalf("missing distinct running image variant: %+v", runningVisual)
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
	runningBinding := binding
	runningBinding.feedbackState = "running"
	runningBinding.feedbackInvocationID = "test-running-invocation"
	runningView, retry := p.commandDeckImageKeyView(a.ctx, runningBinding, "en", model)
	if retry || runningView.State != "running" || !strings.Contains(runningView.ImageID, runningVisual.imageRef) || !strings.Contains(runningView.ImageID, ":state:running") {
		t.Fatalf("running variant was not selected in rendered identity: view=%+v retry=%v", runningView, retry)
	}
	runningPNG, err := commandimage.Load(a.ctx, database.DB(), p.principal.UserID, runningVisual.imageRef)
	if err != nil {
		t.Fatalf("load running variant image: %v", err)
	}
	expectedRunningPixels := commandDeckPresentationImageWithStatus(runningVisual.title, runningVisual.icon, runningPNG, commandDeckFeedbackStatusLabel("en", "running"), model)
	if !bytes.Equal(runningView.ImageRGBA, expectedRunningPixels) || bytes.Equal(runningView.ImageRGBA, view.ImageRGBA) || bytes.Equal(runningView.ImageRGBA, fallback.ImageRGBA) {
		t.Fatal("running variant pixels did not match its stored image or differ from base/fallback")
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
	statePresentation, ok := input.Presentation["states"].(map[string]any)
	if !ok {
		t.Fatal("variant presentation missing after confirmed round trip")
	}
	running, ok := statePresentation["running"].(map[string]any)
	if !ok {
		t.Fatal("running presentation missing after confirmed round trip")
	}
	stateRef, ok := running["image_ref"].(string)
	if !ok || !commandimage.ValidRef(stateRef) || stateRef != runningVisual.imageRef || stateRef == binding.imageRef {
		t.Fatalf("variant image was not independently persisted: %q", stateRef)
	}
	if _, err := commandimage.Load(a.ctx, database.DB(), p.principal.UserID, stateRef); err != nil {
		t.Fatalf("variant image missing for owner: %v", err)
	}
	if _, err := commandimage.Load(a.ctx, database.DB(), "another-user", stateRef); err == nil {
		t.Fatal("variant image crossed owner boundary")
	}
	// An unrelated confirmed edit remains below the unchanged document limit.
	input.Presentation["icon"] = "folder"
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_update", ID: input.ID, Binding: &input})
	delete(input.Presentation, "image_ref")
	delete(running, "image_ref")
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_update", ID: input.ID, Binding: &input})
	if _, err := commandimage.Load(a.ctx, database.DB(), p.principal.UserID, binding.imageRef); err == nil {
		t.Fatal("orphan after removal")
	}
	if _, err := commandimage.Load(a.ctx, database.DB(), p.principal.UserID, stateRef); err == nil {
		t.Fatal("orphan variant image after removal")
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

func TestCommandSettingsImageConfirmedEditRetainsUnavailableImportedReferences(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layer, _ := settingsActivationSecurityLayerAndRule(t, a, decisions)
	input := CommandSettingsBindingInput{LayerID: layer, CommandID: "navigation.settings.open", TriggerType: "streamdeck.key", TriggerSpec: `{"version":1,"device":"test-deck","key":0}`, Effect: "execute", Enabled: true, Presentation: map[string]any{
		"version": 1, "image_upload": settingsImageUpload(t),
		"states": map[string]any{"running": map[string]any{"image_upload": settingsImageUploadSeed(t, 41)}},
	}}
	created := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_create", Binding: &input})
	input.ID = created.ID
	snapshot, err := a.GetCommandSettingsForScope("en", "global")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range snapshot.Bindings {
		if row.ID == input.ID {
			input.Presentation = row.Presentation
		}
	}
	baseRef, ok := input.Presentation["image_ref"].(string)
	if !ok {
		t.Fatal("missing persisted reference")
	}
	// Model a portable binding whose image bytes are unavailable locally.
	// This is the fixture's isolated database, never a user's database.
	owner := a.commandProduct.Load().principal.UserID
	if err := database.DB().Exec("DELETE FROM command_image_assets WHERE user_id = ?", owner).Error; err != nil {
		t.Fatal(err)
	}
	input.Presentation["icon"] = "folder"
	updated := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_update", ID: input.ID, Binding: &input})
	if !updated.Committed || !updated.Published {
		t.Fatalf("unrelated edit failed: %+v", updated)
	}
	snapshot, err = a.GetCommandSettingsForScope("en", "global")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range snapshot.Bindings {
		if row.ID != input.ID {
			continue
		}
		if row.Presentation["icon"] != "folder" || row.Presentation["image_ref"] != baseRef || row.Presentation["states"] == nil {
			t.Fatalf("lost retained presentation: %+v", row.Presentation)
		}
		if _, err := commandimage.Load(a.ctx, database.DB(), owner, baseRef); !errors.Is(err, commandimage.ErrNotFound) {
			t.Fatalf("missing asset unexpectedly created: %v", err)
		}
		return
	}
	t.Fatal("updated binding missing")
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
