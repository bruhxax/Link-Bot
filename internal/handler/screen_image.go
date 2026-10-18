package handler

import (
	"bytes"
	"image"
	"image/jpeg"
	_ "image/png"
	"path/filepath"
	"strings"
)

// Optimize only the copy sent to Telegram. Keep the original asset, dimensions,
// transparent images and unsupported formats intact.
func optimizeScreenImage(data []byte, filename string) ([]byte, string) {
	if len(data) < 256*1024 {
		return data, filename
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "png" && format != "jpeg") || config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 16_000_000 {
		return data, filename
	}
	picture, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return data, filename
	}
	opaque, ok := picture.(interface{ Opaque() bool })
	if !ok || !opaque.Opaque() {
		return data, filename
	}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, picture, &jpeg.Options{Quality: 88}); err != nil || encoded.Len() > len(data)*4/5 {
		return data, filename
	}
	return encoded.Bytes(), strings.TrimSuffix(filename, filepath.Ext(filename)) + ".jpg"
}
