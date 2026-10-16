package commandimage

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"

	"golang.org/x/image/draw"
)

const (
	maxInputBytes  = 1 << 20
	maxImageAxis   = 4096
	maxImagePixels = 4_000_000
	maxImageSide   = 128
)

var (
	ErrInvalidImage      = errors.New("commandimage: imagem inválida")
	ErrInputTooLarge     = errors.New("commandimage: imagem de entrada excede o limite")
	ErrUnsupportedFormat = errors.New("commandimage: formato de imagem não suportado")
	ErrImageTooLarge     = errors.New("commandimage: dimensões da imagem excedem o limite")
)

// Asset é a representação canônica de uma imagem local. PNG contém somente a
// codificação normalizada; Ref é o digest SHA-256 desses mesmos bytes.
type Asset struct {
	Ref string
	PNG []byte
}

// Normalize aceita somente PNG e JPEG e produz um PNG sem metadados, limitado
// a 128 pixels no maior eixo sem ampliar imagens menores.
func Normalize(input []byte) (Asset, error) {
	if len(input) == 0 {
		return Asset{}, ErrInvalidImage
	}
	if len(input) > maxInputBytes {
		return Asset{}, ErrInputTooLarge
	}

	config, format, err := image.DecodeConfig(bytes.NewReader(input))
	if err != nil {
		return Asset{}, fmt.Errorf("%w: ler configuração: %v", ErrInvalidImage, err)
	}
	if format != "png" && format != "jpeg" {
		return Asset{}, ErrUnsupportedFormat
	}
	if !validDimensions(config.Width, config.Height) {
		return Asset{}, ErrImageTooLarge
	}

	decoded, decodedFormat, err := image.Decode(bytes.NewReader(input))
	if err != nil {
		return Asset{}, fmt.Errorf("%w: decodificar: %v", ErrInvalidImage, err)
	}
	if decodedFormat != format || !validDimensions(decoded.Bounds().Dx(), decoded.Bounds().Dy()) {
		return Asset{}, ErrInvalidImage
	}

	normalized := fit(decoded)
	var encoded bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestCompression}
	if err := encoder.Encode(&encoded, normalized); err != nil {
		return Asset{}, fmt.Errorf("%w: codificar PNG: %v", ErrInvalidImage, err)
	}
	pngBytes := encoded.Bytes()
	digest := sha256.Sum256(pngBytes)
	return Asset{
		Ref: hex.EncodeToString(digest[:]),
		PNG: append([]byte(nil), pngBytes...),
	}, nil
}

func validDimensions(width, height int) bool {
	return width > 0 && height > 0 && width <= maxImageAxis && height <= maxImageAxis && int64(width)*int64(height) <= maxImagePixels
}

func fit(src image.Image) image.Image {
	bounds := src.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= maxImageSide && height <= maxImageSide {
		// Canonical storage is always 8-bit RGBA. A small 16-bit PNG must
		// not retain twice the pixel depth and exceed the asset byte limit.
		dst := image.NewNRGBA(image.Rect(0, 0, width, height))
		draw.Draw(dst, dst.Bounds(), src, bounds.Min, draw.Src)
		return dst
	}

	if width >= height {
		width = maxImageSide
		height = max(1, height*maxImageSide/bounds.Dx())
	} else {
		height = maxImageSide
		width = max(1, bounds.Dx()*maxImageSide/bounds.Dy())
	}
	dst := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, bounds, draw.Src, nil)
	return dst
}

// ValidRef reports whether ref is a lowercase hexadecimal SHA-256 digest.
func ValidRef(ref string) bool {
	if len(ref) != sha256.Size*2 {
		return false
	}
	for _, c := range ref {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
