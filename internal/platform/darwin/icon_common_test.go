package darwin

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

func TestVerifyIconPNGRequires32Pixels(t *testing.T) {
	var output bytes.Buffer
	if err := png.Encode(&output, image.NewRGBA(image.Rect(0, 0, 32, 32))); err != nil {
		t.Fatal(err)
	}
	if err := verifyIconPNG(output.Bytes()); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := png.Encode(&output, image.NewRGBA(image.Rect(0, 0, 16, 16))); err != nil {
		t.Fatal(err)
	}
	if err := verifyIconPNG(output.Bytes()); err == nil {
		t.Fatal("expected non-32x32 icon to fail")
	}
}
