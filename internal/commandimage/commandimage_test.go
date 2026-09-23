package commandimage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestNormalizeCanonicalizesSupportedFormats(t *testing.T) {
	src := testImage(256, 64)
	var pngInput bytes.Buffer
	require.NoError(t, png.Encode(&pngInput, src))
	var jpegInput bytes.Buffer
	require.NoError(t, jpeg.Encode(&jpegInput, src, &jpeg.Options{Quality: 90}))

	for name, input := range map[string][]byte{"png": pngInput.Bytes(), "jpeg": jpegInput.Bytes()} {
		t.Run(name, func(t *testing.T) {
			asset, err := Normalize(input)
			require.NoError(t, err)
			require.True(t, ValidRef(asset.Ref))
			require.Len(t, asset.Ref, 64)
			sum := sha256Sum(asset.PNG)
			require.Equal(t, sum, asset.Ref)

			config, format, err := image.DecodeConfig(bytes.NewReader(asset.PNG))
			require.NoError(t, err)
			require.Equal(t, "png", format)
			require.Equal(t, 128, config.Width)
			require.Equal(t, 32, config.Height)
		})
	}
}

func TestNormalizeDoesNotEnlargeAndDropsPNGMetadata(t *testing.T) {
	var input bytes.Buffer
	require.NoError(t, png.Encode(&input, testImage(32, 16)))
	withMetadata := insertPNGTextChunk(input.Bytes(), "secret")

	asset, err := Normalize(withMetadata)
	require.NoError(t, err)
	config, _, err := image.DecodeConfig(bytes.NewReader(asset.PNG))
	require.NoError(t, err)
	require.Equal(t, 32, config.Width)
	require.Equal(t, 16, config.Height)
	require.NotContains(t, asset.PNG, []byte("tEXt"))
	require.NotContains(t, asset.PNG, []byte("secret"))
}

func TestNormalizeConvertsSmall16BitPNGToCanonicalEightBit(t *testing.T) {
	img := image.NewNRGBA64(image.Rect(0, 0, 128, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			img.SetNRGBA64(x, y, color.NRGBA64{R: uint16(x * 509), G: uint16(y * 503), B: uint16((x + y) * 251), A: 65535})
		}
	}
	var data bytes.Buffer
	require.NoError(t, png.Encode(&data, img))
	asset, err := Normalize(data.Bytes())
	require.NoError(t, err)
	require.LessOrEqual(t, len(asset.PNG), maxStoredPNG)
	// The PNG IHDR bit-depth byte is 24, irrespective of the pixel values.
	require.Equal(t, byte(8), asset.PNG[24])
	again, err := Normalize(asset.PNG)
	require.NoError(t, err)
	require.Equal(t, asset, again)
}

func TestNormalizeRejectsMalformedUnsupportedBombAndOversizedInput(t *testing.T) {
	var gifInput bytes.Buffer
	require.NoError(t, gif.Encode(&gifInput, testImage(1, 1), nil))
	_, err := Normalize(gifInput.Bytes())
	require.ErrorIs(t, err, ErrUnsupportedFormat)

	_, err = Normalize([]byte("not an image"))
	require.ErrorIs(t, err, ErrInvalidImage)

	tooWide := testImage(4097, 1)
	var bomb bytes.Buffer
	require.NoError(t, png.Encode(&bomb, tooWide))
	_, err = Normalize(bomb.Bytes())
	require.ErrorIs(t, err, ErrImageTooLarge)

	_, err = Normalize(bytes.Repeat([]byte{'x'}, maxInputBytes+1))
	require.ErrorIs(t, err, ErrInputTooLarge)
}

func TestValidRef(t *testing.T) {
	require.True(t, ValidRef("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"))
	require.False(t, ValidRef(""))
	require.False(t, ValidRef("0123456789ABCDEF0123456789abcdef0123456789abcdef0123456789abcdef"))
	require.False(t, ValidRef("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdeg"))
	require.False(t, ValidRef("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcde"))
}

func TestMigrateIsIndependentAndPutLoadAreOwnerScoped(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	require.NoError(t, Migrate(ctx, db))

	var tableCount int
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'command_image_assets'").Scan(&tableCount).Error)
	require.Equal(t, 1, tableCount)
	var bindingsCount int
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'command_bindings'").Scan(&bindingsCount).Error)
	require.Zero(t, bindingsCount)

	asset := mustAsset(t, 32, 32, color.RGBA{R: 20, G: 40, B: 80, A: 255})
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		require.NoError(t, PutTx(ctx, tx, "owner-a", asset))
		return PutTx(ctx, tx, "owner-a", asset)
	}))

	loaded, err := Load(ctx, db, "owner-a", asset.Ref)
	require.NoError(t, err)
	require.Equal(t, asset.PNG, loaded)
	_, err = Load(ctx, db, "owner-b", asset.Ref)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = Load(ctx, db, "owner-a", "bad-ref")
	require.ErrorIs(t, err, ErrInvalidRef)

	require.NoError(t, db.Exec("UPDATE command_image_assets SET png = ? WHERE user_id = ? AND ref = ?", []byte("corrupt"), "owner-a", asset.Ref).Error)
	_, err = Load(ctx, db, "owner-a", asset.Ref)
	require.ErrorIs(t, err, ErrIntegrity)
}

func TestPutTxRejectsMalformedAssetsAndEnforcesOwnerQuota(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	require.NoError(t, Migrate(ctx, db))
	asset := mustAsset(t, 16, 16, color.RGBA{R: 220, G: 40, B: 80, A: 255})

	badRef := asset
	badRef.Ref = "0000000000000000000000000000000000000000000000000000000000000000"
	require.ErrorIs(t, db.Transaction(func(tx *gorm.DB) error { return PutTx(ctx, tx, "owner-a", badRef) }), ErrIntegrity)

	badPNG := asset
	badPNG.PNG = append([]byte(nil), asset.PNG...)
	badPNG.PNG[len(badPNG.PNG)-1] ^= 1
	require.Error(t, db.Transaction(func(tx *gorm.DB) error { return PutTx(ctx, tx, "owner-a", badPNG) }))

	fullBlob := bytes.Repeat([]byte{'x'}, maxStoredPNG)
	for i := 0; i <= ownerQuota/maxStoredPNG; i++ {
		ref := fmt.Sprintf("%064x", i+1)
		require.NoError(t, db.Exec("INSERT INTO command_image_assets (user_id, ref, png) VALUES (?, ?, ?)", "owner-a", ref, fullBlob).Error)
	}
	require.ErrorIs(t, db.Transaction(func(tx *gorm.DB) error { return PutTx(ctx, tx, "owner-a", asset) }), ErrQuotaExceeded)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return PutTx(ctx, tx, "owner-b", asset) }))

	require.ErrorIs(t, db.Transaction(func(tx *gorm.DB) error { return PutTx(ctx, tx, "   ", asset) }), ErrInvalidOwner)
}

func TestPruneTxKeepsReferencesAcrossWorkspacesAndOwners(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	require.NoError(t, Migrate(ctx, db))
	require.NoError(t, db.Exec(`CREATE TABLE command_bindings (
		user_id TEXT NOT NULL,
		workspace_id TEXT,
		presentation TEXT NOT NULL
	)`).Error)

	kept := mustAsset(t, 20, 20, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	removed := mustAsset(t, 20, 20, color.RGBA{R: 40, G: 50, B: 60, A: 255})
	otherOwner := mustAsset(t, 20, 20, color.RGBA{R: 70, G: 80, B: 90, A: 255})
	for _, item := range []struct {
		owner string
		asset Asset
	}{
		{"owner-a", kept}, {"owner-a", removed}, {"owner-b", otherOwner},
	} {
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return PutTx(ctx, tx, item.owner, item.asset) }))
	}

	require.NoError(t, db.Exec("INSERT INTO command_bindings (user_id, workspace_id, presentation) VALUES (?, ?, ?)", "owner-a", "workspace-1", fmt.Sprintf(`{"image_ref":%q}`, kept.Ref)).Error)
	require.NoError(t, db.Exec("INSERT INTO command_bindings (user_id, workspace_id, presentation) VALUES (?, ?, ?)", "owner-a", nil, `{"title":"global"}`).Error)
	require.NoError(t, db.Exec("INSERT INTO command_bindings (user_id, workspace_id, presentation) VALUES (?, ?, ?)", "owner-b", "workspace-2", fmt.Sprintf(`{"image_ref":%q}`, otherOwner.Ref)).Error)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return PruneTx(ctx, tx, "owner-a") }))
	_, err := Load(ctx, db, "owner-a", kept.Ref)
	require.NoError(t, err)
	_, err = Load(ctx, db, "owner-a", removed.Ref)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = Load(ctx, db, "owner-b", otherOwner.Ref)
	require.NoError(t, err)
}

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "commandimage.db")), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func testImage(width, height int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x % 251), G: uint8(y % 251), B: uint8((x + y) % 251), A: 255})
		}
	}
	return img
}

func mustAsset(t *testing.T, width, height int, fill color.RGBA) Asset {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, fill)
		}
	}
	var encoded bytes.Buffer
	require.NoError(t, png.Encode(&encoded, img))
	asset, err := Normalize(encoded.Bytes())
	require.NoError(t, err)
	return asset
}

func sha256Sum(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func insertPNGTextChunk(input []byte, text string) []byte {
	chunkData := append([]byte("Comment\x00"), []byte(text)...)
	chunk := make([]byte, 12+len(chunkData))
	binary.BigEndian.PutUint32(chunk[:4], uint32(len(chunkData)))
	copy(chunk[4:8], "tEXt")
	copy(chunk[8:8+len(chunkData)], chunkData)
	binary.BigEndian.PutUint32(chunk[8+len(chunkData):], crc32.ChecksumIEEE(chunk[4:8+len(chunkData)]))
	insertAt := len(input) - 12
	return append(append(append([]byte(nil), input[:insertAt]...), chunk...), input[insertAt:]...)
}
