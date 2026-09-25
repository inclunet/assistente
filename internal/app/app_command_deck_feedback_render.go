package app

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"strings"

	"assistente/internal/commanddeck"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// commandDeckFeedbackStatusLabel is deliberately local to the physical
// surface. The backend state remains stable and the Stream Deck only receives
// the short, localized presentation of it.
func commandDeckFeedbackStatusLabel(locale, state string) string {
	labels := map[string]map[string]string{
		"pt-BR": {
			"on":              "Ligado",
			"off":             "Desligado",
			"waiting":         "Aguardando",
			"running":         "Em execução",
			"succeeded":       "Concluído",
			"failed":          "Falhou",
			"denied":          "Negado",
			"cancelled":       "Cancelado",
			"timed_out":       "Tempo esgotado",
			"outcome_unknown": "Resultado desconhecido",
		},
		"en": {
			"on":              "On",
			"off":             "Off",
			"waiting":         "Waiting",
			"running":         "Running",
			"succeeded":       "Succeeded",
			"failed":          "Failed",
			"denied":          "Denied",
			"cancelled":       "Cancelled",
			"timed_out":       "Timed out",
			"outcome_unknown": "Outcome unknown",
		},
		"es": {
			"on":              "Activado",
			"off":             "Desactivado",
			"waiting":         "En espera",
			"running":         "En ejecución",
			"succeeded":       "Completado",
			"failed":          "Fallido",
			"denied":          "Denegado",
			"cancelled":       "Cancelado",
			"timed_out":       "Tiempo agotado",
			"outcome_unknown": "Resultado desconocido",
		},
	}
	if locale != "pt-BR" && locale != "es" {
		locale = "en"
	}
	return labels[locale][state]
}

// cloneCommandDeckMap makes the feedback projection disposable. In
// particular, a frame from a retired Stream Deck instance cannot leave status
// fields behind in the fresh map used for the next comparison.
func cloneCommandDeckMap(source commandDeckMap) commandDeckMap {
	result := make(commandDeckMap, len(source))
	for serial, keys := range source {
		result[serial] = make(map[int]commandDeckBinding, len(keys))
		for index, binding := range keys {
			result[serial][index] = binding
		}
	}
	return result
}

// overlayDeckFeedback adds status only for the controller's current live
// instance. The instance snapshot is copied while holding the controller lock;
// the potentially slower backend lookup happens after releasing it.
func (c *commandDeckController) overlayDeckFeedback(ctx context.Context, source commandDeckMap) commandDeckMap {
	result := cloneCommandDeckMap(source)
	for _, keys := range result {
		for index, binding := range keys {
			binding.feedbackState = ""
			binding.feedbackInvocationID = ""
			keys[index] = binding
		}
	}
	if c == nil || c.p == nil || ctx == nil || ctx.Err() != nil {
		return result
	}
	c.mu.Lock()
	instances := make(map[string]commandDeckInstance, len(c.instances))
	for serial, instance := range c.instances {
		if instance.ctx == nil || instance.ctx.Err() != nil {
			continue
		}
		instances[serial] = instance
	}
	versions, generation := c.versions, c.generation
	c.mu.Unlock()

	for serial, keys := range result {
		instance, live := instances[serial]
		for index, binding := range keys {
			if live && binding.identity != "" && ctx.Err() == nil {
				binding.feedbackState, binding.feedbackInvocationID = c.p.deckFeedbackSnapshot(binding.identity, instance.id, versions, generation)
			}
			keys[index] = binding
		}
	}
	return result
}

// commandDeckPresentationImageWithStatus reserves a small footer before
// laying out the title. Existing title, custom image and icon placement remain
// the primary content; the footer is the only area used for status text.
func commandDeckPresentationImageWithStatus(title, icon string, customPNG []byte, status string, model commanddeck.Model) []byte {
	img := image.NewRGBA(image.Rect(0, 0, model.KeyImageW, model.KeyImageH))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{24, 24, 24, 255}), image.Point{}, draw.Src)
	contentBottom, footerHeight := commandDeckPresentationStatusLayout(model, status)
	y := 14
	custom := commandDeckDecodeImage(customPNG)
	if (custom != nil || commandDeckIconSupported(icon)) && model.KeyImageW >= 20 && contentBottom >= 40 {
		size := min(32, model.KeyImageW-4, contentBottom/2-4)
		x := (model.KeyImageW - size) / 2
		if custom != nil {
			commandDeckDrawImage(img, custom, image.Rect(x, 2, x+size, 2+size))
		} else {
			commandDeckDrawIcon(img, icon, image.Rect(x, 2, x+size, 2+size))
		}
		y += size + 4
	}
	parsed, err := opentype.Parse(goregular.TTF)
	if err == nil {
		face, faceErr := opentype.NewFace(parsed, &opentype.FaceOptions{Size: 11, DPI: 72, Hinting: font.HintingFull})
		if faceErr == nil {
			defer func() {
				// A renderização já foi produzida e esta API não retorna erro de cleanup.
				_ = face.Close()
			}()
			drawer := font.Drawer{Dst: img, Src: image.White, Face: face}
			titleBottom := contentBottom - 3
			titleBaseline := min(14, titleBottom)
			if y != 14 {
				titleBaseline = min(y, titleBottom)
			}
			line := ""
			if titleBaseline < 0 {
				return img.Pix
			}
			y = titleBaseline
			for _, word := range strings.Fields(title) {
				candidate := strings.TrimSpace(line + " " + word)
				if drawer.MeasureString(candidate).Ceil() > model.KeyImageW-4 && line != "" {
					if y > titleBottom {
						break
					}
					drawer.Dot = fixed.P(2, y)
					drawer.DrawString(line)
					y += 14
					line = word
				} else {
					line = candidate
				}
				if y > titleBottom {
					break
				}
			}
			if line != "" && y <= titleBottom {
				drawer.Dot = fixed.P(2, y)
				drawer.DrawString(line)
			}

			if footerHeight != 0 {
				footerFace, footerErr := opentype.NewFace(parsed, &opentype.FaceOptions{Size: 8, DPI: 72, Hinting: font.HintingFull})
				if footerErr == nil {
					defer func() {
						// A renderização já foi produzida e esta API não retorna erro de cleanup.
						_ = footerFace.Close()
					}()
					footer := font.Drawer{Dst: img, Src: image.White, Face: footerFace}
					footerText := fitCommandDeckText(footer, status, model.KeyImageW-4)
					footer.Dot = fixed.P(2, model.KeyImageH-3)
					footer.DrawString(footerText)
				}
			}
		}
	}
	return img.Pix
}

func fitCommandDeckText(drawer font.Drawer, value string, maxWidth int) string {
	if drawer.MeasureString(value).Ceil() <= maxWidth {
		return value
	}
	const ellipsis = "…"
	runes := []rune(value)
	for len(runes) > 0 {
		runes = []rune(strings.TrimSpace(string(runes[:len(runes)-1])))
		candidate := string(runes) + ellipsis
		if drawer.MeasureString(candidate).Ceil() <= maxWidth {
			return candidate
		}
	}
	return ellipsis
}

func commandDeckPresentationStatusLayout(model commanddeck.Model, status string) (contentBottom, footerHeight int) {
	contentBottom = model.KeyImageH
	if strings.TrimSpace(status) == "" {
		return contentBottom, 0
	}
	const statusFooterHeight = 16
	const minimumTitleHeight = 18
	if model.KeyImageH < statusFooterHeight+minimumTitleHeight {
		// The title is primary content. State and Announce still carry the
		// status when the physical raster is too small for both regions.
		return contentBottom, 0
	}
	return model.KeyImageH - statusFooterHeight, statusFooterHeight
}
