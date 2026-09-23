package app

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"

	"assistente/internal/commanddeck"
	"assistente/internal/commandimage"
	"assistente/internal/database"
	xdraw "golang.org/x/image/draw"
)

// Called only for changed frames or device reconnection, never on key-down.
// No cross-session image cache, file paths, URLs or caller-supplied owner IDs.
func (p *commandProductRuntime) commandDeckImageKeyView(ctx context.Context, binding commandDeckBinding, locale string, model commanddeck.Model) (commanddeck.KeyView, bool) {
	binding = commandDeckPresentedBinding(binding)
	retry := false
	if binding.imageRef != "" && ctx.Err() == nil && p.app.commandProduct.Load() == p && p.dependenciesMatch(p.app) {
		var err error
		binding.imagePNG, err = commandimage.Load(ctx, database.DB(), p.principal.UserID, binding.imageRef)
		// Missing/imported or corrupt assets have a stable fallback. Storage
		// failures may recover without a configuration change or reconnect.
		retry = err != nil && ctx.Err() == nil && !errors.Is(err, commandimage.ErrNotFound) &&
			!errors.Is(err, commandimage.ErrIntegrity) && !errors.Is(err, commandimage.ErrInvalidRef) && !errors.Is(err, commandimage.ErrInvalidOwner)
	}
	return commandDeckKeyView(binding, locale, model), retry
}

func commandDeckDecodeImage(data []byte) image.Image {
	if len(data) == 0 || len(data) > 80<<10 {
		return nil
	}
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 128 || config.Height > 128 {
		return nil
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	return img
}

func commandDeckDrawImage(dst *image.RGBA, src image.Image, box image.Rectangle) {
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	if w*box.Dy() > h*box.Dx() {
		h = max(1, h*box.Dx()/w)
		w = box.Dx()
	} else {
		w = max(1, w*box.Dy()/h)
		h = box.Dy()
	}
	x, y := box.Min.X+(box.Dx()-w)/2, box.Min.Y+(box.Dy()-h)/2
	xdraw.CatmullRom.Scale(dst, image.Rect(x, y, x+w, y+h), src, src.Bounds(), xdraw.Over, nil)
}
