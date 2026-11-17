//go:build ignore

// gen_icons generates PWA icon PNGs for MixerSmash.
// Run from the repo root:
//
//	go run tools/gen_icons/main.go
//
// Output: web/icon-192.png and web/icon-512.png
//
// Each icon is a solid dark background (#101014) filling the square, with a
// centred teal/blue filled circle (~60% of the icon size) as a nod to the
// gel buttons in the game.
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
)

func main() {
	if err := writeIcon("web/icon-192.png", 192); err != nil {
		log.Fatalf("icon-192.png: %v", err)
	}
	if err := writeIcon("web/icon-512.png", 512); err != nil {
		log.Fatalf("icon-512.png: %v", err)
	}
	log.Println("wrote web/icon-192.png and web/icon-512.png")
}

// writeIcon creates a size×size PNG at path with a dark background and a
// centred teal circle at 60% of the icon size.
func writeIcon(path string, size int) error {
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	bg := color.RGBA{R: 0x10, G: 0x10, B: 0x14, A: 0xff}
	circle := color.RGBA{R: 0x1a, G: 0xb5, B: 0xc8, A: 0xff}

	// Fill background.
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.Set(x, y, bg)
		}
	}

	// Draw filled circle centred in the image, radius = 30% of size.
	cx := float64(size) / 2.0
	cy := float64(size) / 2.0
	r := float64(size) * 0.30

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx := float64(x) + 0.5 - cx
			dy := float64(y) + 0.5 - cy
			if math.Sqrt(dx*dx+dy*dy) <= r {
				img.Set(x, y, circle)
			}
		}
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return png.Encode(f, img)
}
