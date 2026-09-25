package app

import (
	"image"
	"image/color"
	"math"

	"golang.org/x/image/vector"
)

// These fixed, local shapes supplement the title; no path, URL, font glyph or
// user-supplied vector is interpreted. Unknown tokens keep the text-only view.
func commandDeckIconSupported(icon string) bool {
	switch icon {
	case "settings", "chat", "folder", "play", "stop", "back", "star":
		return true
	}
	return false
}

func commandDeckDrawIcon(dst *image.RGBA, icon string, bounds image.Rectangle) {
	polygon := func(points [][2]float64, foreground bool) {
		if len(points) < 3 {
			return
		}
		raster := vector.NewRasterizer(dst.Bounds().Dx(), dst.Bounds().Dy())
		for i, point := range points {
			x := float32(float64(bounds.Min.X) + point[0]*float64(bounds.Dx()))
			y := float32(float64(bounds.Min.Y) + point[1]*float64(bounds.Dy()))
			if i == 0 {
				raster.MoveTo(x, y)
			} else {
				raster.LineTo(x, y)
			}
		}
		raster.ClosePath()
		ink := image.White
		if !foreground {
			ink = image.NewUniform(color.RGBA{24, 24, 24, 255})
		}
		raster.Draw(dst, dst.Bounds(), ink, image.Point{})
	}
	switch icon {
	case "chat":
		polygon([][2]float64{{.05, .08}, {.95, .08}, {.95, .72}, {.4, .72}, {.16, .95}, {.16, .72}, {.05, .72}}, true)
		polygon([][2]float64{{.16, .2}, {.84, .2}, {.84, .58}, {.16, .58}}, false)
	case "folder":
		polygon([][2]float64{{.04, .14}, {.4, .14}, {.53, .3}, {.96, .3}, {.96, .88}, {.04, .88}}, true)
	case "play":
		polygon([][2]float64{{.22, .08}, {.92, .5}, {.22, .92}}, true)
	case "stop":
		polygon([][2]float64{{.12, .12}, {.88, .12}, {.88, .88}, {.12, .88}}, true)
	case "back":
		polygon([][2]float64{{.04, .5}, {.5, .04}, {.5, .32}, {.96, .32}, {.96, .68}, {.5, .68}, {.5, .96}}, true)
	case "star", "settings":
		count := 10
		if icon == "settings" {
			count = 32
		}
		points := make([][2]float64, count)
		for i := range points {
			radius := .48
			if icon == "star" && i%2 == 1 {
				radius = .21
			}
			if icon == "settings" && i%4 >= 2 {
				radius = .35
			}
			angle := float64(i)*2*math.Pi/float64(count) - math.Pi/2
			points[i] = [2]float64{.5 + radius*math.Cos(angle), .5 + radius*math.Sin(angle)}
		}
		polygon(points, true)
		if icon == "settings" {
			for i := range points {
				angle := float64(i) * 2 * math.Pi / float64(count)
				points[i] = [2]float64{.5 + .17*math.Cos(angle), .5 + .17*math.Sin(angle)}
			}
			polygon(points, false)
		}
	}
}
