package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"path/filepath"
	"testing"

	"assistente/internal/commandconfig"
	"assistente/internal/commandexecution"
	"assistente/internal/commandimage"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func revisionImageDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "image-revision.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, commandimage.Migrate(context.Background(), db))
	require.NoError(t, db.Exec(`CREATE TABLE command_bindings (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, workspace_id TEXT, presentation TEXT NOT NULL)`).Error)
	return db
}

func revisionImageAsset(t *testing.T, red uint8) commandimage.Asset {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: red, G: uint8(x), B: uint8(y), A: 255})
		}
	}
	var encoded bytes.Buffer
	require.NoError(t, png.Encode(&encoded, img))
	asset, err := commandimage.Normalize(encoded.Bytes())
	require.NoError(t, err)
	return asset
}

func revisionImagePresentation(base, state string) string {
	return fmt.Sprintf(`{"version":1,"icon":"star","image_ref":%q,"states":{"running":{"image_ref":%q}}}`, base, state)
}

func revisionImageInsertBinding(t *testing.T, db *gorm.DB, binding commandconfig.Binding) {
	t.Helper()
	require.NoError(t, db.Exec("INSERT INTO command_bindings (id, user_id, workspace_id, presentation) VALUES (?, ?, ?, ?)", binding.ID, binding.UserID, binding.WorkspaceID, binding.Presentation).Error)
}

// The production hook runs after the new binding is persisted, in the same
// transaction. Keep that order here so pruning observes the replacement.
func revisionImageCommit(db *gorm.DB, binding commandconfig.Binding, batch *commandSettingsImageBatch, before []commandconfig.Binding, failAfter error) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("UPDATE command_bindings SET presentation = ? WHERE id = ? AND user_id = ?", binding.Presentation, binding.ID, binding.UserID).Error; err != nil {
			return err
		}
		intent := commandconfig.MutationIntent{Operation: commandconfig.BindingUpdate, ID: binding.ID, Binding: &binding}
		if err := commitCommandSettingsImage(context.Background(), tx, binding.UserID, intent, batch, before); err != nil {
			return err
		}
		return failAfter
	})
}

func TestCommandSettingsImageRevisionReplaceAtQuotaAndRollback(t *testing.T) {
	for _, rollback := range []bool{false, true} {
		t.Run(fmt.Sprintf("rollback=%v", rollback), func(t *testing.T) {
			ctx := context.Background()
			db := revisionImageDB(t)
			oldBase, oldState := revisionImageAsset(t, 10), revisionImageAsset(t, 20)
			newBase, newState := revisionImageAsset(t, 30), revisionImageAsset(t, 40)
			keptBase, keptState := revisionImageAsset(t, 50), revisionImageAsset(t, 60)
			for _, asset := range []commandimage.Asset{oldBase, oldState, keptBase, keptState} {
				require.NoError(t, commandimage.PutTx(ctx, db, "owner-a", asset))
			}
			require.NoError(t, commandimage.PutTx(ctx, db, "owner-b", oldBase))
			before := commandconfig.Binding{ID: "edited", UserID: "owner-a", Presentation: revisionImagePresentation(oldBase.Ref, oldState.Ref)}
			revisionImageInsertBinding(t, db, before)
			workspace := "other-workspace"
			revisionImageInsertBinding(t, db, commandconfig.Binding{ID: "retained", UserID: "owner-a", WorkspaceID: &workspace, Presentation: revisionImagePresentation(keptBase.Ref, keptState.Ref)})
			// Quota-only fixture blobs respect the storage byte limit. They are
			// unreferenced and never decoded; retained images above are real PNGs.
			var originalBytes int64
			require.NoError(t, db.Raw("SELECT SUM(length(png)) FROM command_image_assets WHERE user_id = ?", "owner-a").Scan(&originalBytes).Error)
			require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
				for remaining, i := (16<<20)-int(originalBytes), 1; remaining > 0; i++ {
					size := min(80<<10, remaining)
					if err := tx.Exec("INSERT INTO command_image_assets (user_id, ref, png) VALUES (?, ?, ?)", "owner-a", fmt.Sprintf("%064x", i), bytes.Repeat([]byte{'x'}, size)).Error; err != nil {
						return err
					}
					remaining -= size
				}
				return nil
			}))
			require.ErrorIs(t, commandimage.PutTx(ctx, db, "owner-a", newBase), commandimage.ErrQuotaExceeded)
			var beforeCount int64
			require.NoError(t, db.Table("command_image_assets").Count(&beforeCount).Error)
			after := before
			after.Presentation = revisionImagePresentation(newBase.Ref, newState.Ref)
			var injected error
			if rollback {
				injected = errors.New("failure after image hook")
			}
			err := revisionImageCommit(db, after, &commandSettingsImageBatch{assets: []commandimage.Asset{newBase, newState}}, []commandconfig.Binding{before}, injected)
			var persisted string
			require.NoError(t, db.Raw("SELECT presentation FROM command_bindings WHERE id = ?", before.ID).Scan(&persisted).Error)
			if rollback {
				require.ErrorIs(t, err, injected)
				require.Equal(t, before.Presentation, persisted)
				var count, total int64
				require.NoError(t, db.Table("command_image_assets").Count(&count).Error)
				require.Equal(t, beforeCount, count)
				require.NoError(t, db.Raw("SELECT SUM(length(png)) FROM command_image_assets WHERE user_id = ?", "owner-a").Scan(&total).Error)
				require.EqualValues(t, 16<<20, total)
			} else {
				require.NoError(t, err)
				require.Equal(t, after.Presentation, persisted)
			}
			for _, pair := range [][2]commandimage.Asset{{oldBase, newBase}, {oldState, newState}} {
				present, absent := pair[1], pair[0]
				if rollback {
					present, absent = pair[0], pair[1]
				}
				loaded, err := commandimage.Load(ctx, db, "owner-a", present.Ref)
				require.NoError(t, err)
				require.Equal(t, present.PNG, loaded)
				_, err = commandimage.Load(ctx, db, "owner-a", absent.Ref)
				require.ErrorIs(t, err, commandimage.ErrNotFound)
			}
			for _, asset := range []commandimage.Asset{keptBase, keptState} {
				loaded, err := commandimage.Load(ctx, db, "owner-a", asset.Ref)
				require.NoError(t, err)
				require.Equal(t, asset.PNG, loaded)
			}
			loaded, err := commandimage.Load(ctx, db, "owner-b", oldBase.Ref)
			require.NoError(t, err)
			require.Equal(t, oldBase.PNG, loaded)
		})
	}
}

func TestCommandSettingsImageRevisionImportedMissingReferenceBoundary(t *testing.T) {
	for _, location := range []string{"base", "state"} {
		for _, scenario := range []string{"unchanged", "other-binding", "other-owner", "new-missing", "new-foreign", "corrupt"} {
			t.Run(location+"/"+scenario, func(t *testing.T) {
				ctx := context.Background()
				db := revisionImageDB(t)
				missing, upload, orphan := revisionImageAsset(t, 70), revisionImageAsset(t, 80), revisionImageAsset(t, 90)
				require.NoError(t, commandimage.PutTx(ctx, db, "owner-a", orphan))
				before := commandconfig.Binding{ID: "edited", UserID: "owner-a", Presentation: `{"version":1,"icon":"star"}`}
				missingDoc := fmt.Sprintf(`{"version":1,"image_ref":%q}`, missing.Ref)
				if location == "state" {
					missingDoc = fmt.Sprintf(`{"version":1,"states":{"running":{"image_ref":%q}}}`, missing.Ref)
				}
				if scenario == "unchanged" || scenario == "corrupt" {
					before.Presentation = missingDoc
				}
				revisionImageInsertBinding(t, db, before)
				snapshot := []commandconfig.Binding{before}
				if scenario == "other-binding" || scenario == "other-owner" {
					unrelated := before
					unrelated.Presentation = missingDoc
					if scenario == "other-binding" {
						unrelated.ID = "another-binding"
					} else {
						unrelated.UserID = "owner-b"
					}
					// Put the unrelated row first so an ID-only or owner-only
					// lookup cannot accidentally pass this boundary test.
					snapshot = []commandconfig.Binding{unrelated, before}
				}
				if scenario == "new-foreign" {
					require.NoError(t, commandimage.PutTx(ctx, db, "owner-b", missing))
				}
				if scenario == "corrupt" {
					require.NoError(t, db.Exec("INSERT INTO command_image_assets (user_id, ref, png) VALUES (?, ?, ?)", "owner-a", missing.Ref, []byte("corrupt")).Error)
				}
				after := before
				after.Presentation = revisionImagePresentation(missing.Ref, upload.Ref)
				if location == "state" {
					after.Presentation = revisionImagePresentation(upload.Ref, missing.Ref)
				}
				err := revisionImageCommit(db, after, &commandSettingsImageBatch{assets: []commandimage.Asset{upload}}, snapshot, nil)
				var persisted string
				require.NoError(t, db.Raw("SELECT presentation FROM command_bindings WHERE id = ?", before.ID).Scan(&persisted).Error)
				if scenario == "unchanged" {
					require.NoError(t, err)
					require.Equal(t, after.Presentation, persisted)
					loaded, err := commandimage.Load(ctx, db, "owner-a", upload.Ref)
					require.NoError(t, err)
					require.Equal(t, upload.PNG, loaded)
				} else {
					require.ErrorIs(t, err, commandexecution.ErrInvalidRequest)
					require.Equal(t, before.Presentation, persisted)
					_, err = commandimage.Load(ctx, db, "owner-a", upload.Ref)
					require.ErrorIs(t, err, commandimage.ErrNotFound)
					loaded, err := commandimage.Load(ctx, db, "owner-a", orphan.Ref)
					require.NoError(t, err, "prune must roll back too")
					require.Equal(t, orphan.PNG, loaded)
				}
				if scenario != "corrupt" {
					_, err = commandimage.Load(ctx, db, "owner-a", missing.Ref)
					require.ErrorIs(t, err, commandimage.ErrNotFound, "missing/foreign bytes must never be copied")
				}
				if scenario == "new-foreign" {
					loaded, err := commandimage.Load(ctx, db, "owner-b", missing.Ref)
					require.NoError(t, err)
					require.Equal(t, missing.PNG, loaded)
				}
			})
		}
	}
}
