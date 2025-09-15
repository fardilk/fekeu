package ocr

import (
	"image/color"
	"os"
	"testing"

	"github.com/disintegration/imaging"
)

func TestErrNoAmount(t *testing.T) {
	img := imaging.New(400, 200, color.NRGBA{255, 255, 255, 255})
	f, err := os.CreateTemp("", "blank-*.png")
	if err != nil {
		t.Skip("temp file")
	}
	_ = f.Close()
	_ = imaging.Save(img, f.Name())
	defer os.Remove(f.Name())
	_, _, _, er := ExtractAmountFromImage(f.Name())
	if er == nil || er != ErrNoAmount {
		t.Fatalf("expected ErrNoAmount got %v", er)
	}
}
