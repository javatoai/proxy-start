package darwin

import (
	"bytes"
	"errors"
	"image/png"
)

func verifyIconPNG(data []byte) error {
	image, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return err
	}
	if image.Bounds().Dx() != 32 || image.Bounds().Dy() != 32 {
		return errors.New("应用图标必须是 32x32 PNG")
	}
	return nil
}
