package handler

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"testing"
)

func TestOptimizeScreenImagePreservesDimensionsAndTransparency(t *testing.T) {
	picture := image.NewNRGBA(image.Rect(0, 0, 800, 400))
	random := rand.New(rand.NewSource(1))
	for y := 0; y < 400; y++ {
		for x := 0; x < 800; x++ {
			picture.SetNRGBA(x, y, color.NRGBA{R: uint8(random.Intn(256)), G: uint8(random.Intn(256)), B: uint8(random.Intn(256)), A: 255})
		}
	}
	var original bytes.Buffer
	if err := png.Encode(&original, picture); err != nil {
		t.Fatal(err)
	}
	optimized, filename := optimizeScreenImage(original.Bytes(), "banner.png")
	if filename != "banner.jpg" || len(optimized) >= len(original.Bytes())*4/5 {
		t.Fatal("large opaque banner was not compressed")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(optimized))
	if err != nil || format != "jpeg" || config.Width != 800 || config.Height != 400 {
		t.Fatalf("changed dimensions or invalid output: %+v %s %v", config, format, err)
	}
	t.Logf("banner upload: %d -> %d bytes", original.Len(), len(optimized))
	picture.SetNRGBA(0, 0, color.NRGBA{A: 0})
	original.Reset()
	if err := png.Encode(&original, picture); err != nil {
		t.Fatal(err)
	}
	untouched, name := optimizeScreenImage(original.Bytes(), "transparent.png")
	if name != "transparent.png" || !bytes.Equal(untouched, original.Bytes()) {
		t.Fatal("transparent banner was changed")
	}
}

func TestOptimizeScreenImageKeepsSmallOrUnknownFiles(t *testing.T) {
	for _, data := range [][]byte{[]byte("small image"), bytes.Repeat([]byte("unknown format"), 25000)} {
		result, name := optimizeScreenImage(data, "banner.webp")
		if name != "banner.webp" || !bytes.Equal(result, data) {
			t.Fatal("unsupported or small asset changed")
		}
	}
}
