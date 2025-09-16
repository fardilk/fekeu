package ocr

import (
	"image"
	"log"
	"os"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/otiai10/gosseract/v2"
)

// runAllOCRPasses executes the multi-pass OCR strategy and returns variant texts and aggregate.
func runAllOCRPasses(path string) (map[string]string, error) {
	out := map[string]string{}
	img, err := imaging.Open(path)
	if err != nil {
		return nil, err
	}
	gray := imaging.Grayscale(img)
	gray = imaging.AdjustContrast(gray, 15)
	gray = imaging.Sharpen(gray, 0.7)
	if gray.Bounds().Dy() < 900 {
		gray = imaging.Resize(gray, 0, 1300, imaging.Lanczos)
	}
	gray = binarize(gray, 210)
	adv := adaptiveThreshold(gray, 15, 7)
	adv = dilate(adv, 1)

	tmpFile, err := os.CreateTemp("", "ocr-base-*.png")
	tmp := path
	if err == nil {
		tmp = tmpFile.Name()
		_ = tmpFile.Close()
		_ = imaging.Save(gray, tmp)
	}

	baseClient := gosseract.NewClient()
	defer baseClient.Close()
	_ = baseClient.SetLanguage("eng")
	_ = baseClient.SetWhitelist("0123456789RpIDRidri.,:()/- ")
	baseClient.SetImage(tmp)
	text, _ := baseClient.Text()
	text = normalizeOCRText(text)
	out["text"] = text

	digitClient := gosseract.NewClient()
	defer digitClient.Close()
	_ = digitClient.SetLanguage("eng")
	_ = digitClient.SetWhitelist("0123456789., ")
	digitClient.SetImage(tmp)
	textDigits, _ := digitClient.Text()
	textDigits = normalizeOCRText(textDigits)
	out["textDigits"] = textDigits

	origClient := gosseract.NewClient()
	defer origClient.Close()
	_ = origClient.SetLanguage("eng")
	_ = origClient.SetWhitelist("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyzRpIDRidri.,:()/- ")
	origClient.SetImage(path)
	textOrig, _ := origClient.Text()
	textOrig = normalizeOCRText(textOrig)
	out["textOrig"] = textOrig

	// Top half passes
	half := gray.Bounds().Dy() / 2
	var textTop, textTopDigits string
	if half > 50 {
		crop := imaging.Crop(gray, image.Rect(0, 0, gray.Bounds().Dx(), half))
		if tmpTop, _ := os.CreateTemp("", "ocr-top-*.png"); tmpTop != nil {
			_ = tmpTop.Close()
			_ = imaging.Save(crop, tmpTop.Name())
			cl := gosseract.NewClient()
			_ = cl.SetLanguage("eng")
			_ = cl.SetWhitelist("0123456789RpIDRidri.,:()/- ")
			cl.SetImage(tmpTop.Name())
			tt, _ := cl.Text()
			cl.Close()
			textTop = normalizeOCRText(tt)
			cl2 := gosseract.NewClient()
			_ = cl2.SetLanguage("eng")
			_ = cl2.SetWhitelist("0123456789., ")
			cl2.SetImage(tmpTop.Name())
			td, _ := cl2.Text()
			cl2.Close()
			textTopDigits = normalizeOCRText(td)
			_ = os.Remove(tmpTop.Name())
		}
	}
	out["textTop"] = textTop
	out["textTopDigits"] = textTopDigits

	// Inverted pass added to textOrig
	inv := imaging.Invert(gray)
	if tmpInv, _ := os.CreateTemp("", "ocr-inv-*.png"); tmpInv != nil {
		_ = tmpInv.Close()
		_ = imaging.Save(inv, tmpInv.Name())
		cliInv := gosseract.NewClient()
		_ = cliInv.SetLanguage("eng")
		_ = cliInv.SetWhitelist("0123456789RpIDRidri.,:()/- ")
		cliInv.SetImage(tmpInv.Name())
		invText, _ := cliInv.Text()
		cliInv.Close()
		_ = os.Remove(tmpInv.Name())
		textOrig += " " + normalizeOCRText(invText)
		out["textOrig"] = textOrig
	}

	variants := []string{text, textDigits, textOrig, textTop, textTopDigits}

	// Advanced preprocessed OCR
	if tmpAdv, _ := os.CreateTemp("", "ocr-adv-*.png"); tmpAdv != nil {
		_ = tmpAdv.Close()
		_ = imaging.Save(adv, tmpAdv.Name())
		cl := gosseract.NewClient()
		_ = cl.SetLanguage("eng")
		_ = cl.SetWhitelist("0123456789RpIDRidri.,:()/- ")
		cl.SetImage(tmpAdv.Name())
		if t, er := cl.Text(); er == nil {
			variants = append(variants, normalizeOCRText(t))
		}
		cl.Close()
		_ = os.Remove(tmpAdv.Name())
	}

	// Multi-PSM passes
	psmModes := []gosseract.PageSegMode{gosseract.PSM_SINGLE_BLOCK, gosseract.PSM_SINGLE_LINE, gosseract.PSM_SPARSE_TEXT, gosseract.PSM_SPARSE_TEXT_OSD}
	for _, mode := range psmModes {
		cl := gosseract.NewClient()
		_ = cl.SetLanguage("eng")
		_ = cl.SetWhitelist("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyzRpIDRidri.,:()/- ")
		_ = cl.SetPageSegMode(mode)
		cl.SetImage(path)
		if t, er := cl.Text(); er == nil {
			variants = append(variants, normalizeOCRText(t))
		}
		cl.Close()
	}

	// Vertical slices
	cols := 4
	W := gray.Bounds().Dx()
	H := gray.Bounds().Dy()
	colW := W / cols
	for i := 0; i < cols; i++ {
		x0 := i * colW
		x1 := x0 + colW
		if i == cols-1 {
			x1 = W
		}
		crop := imaging.Crop(gray, image.Rect(x0, 0, x1, H))
		if tmpSlice, _ := os.CreateTemp("", "ocr-slice-*.png"); tmpSlice != nil {
			_ = tmpSlice.Close()
			_ = imaging.Save(crop, tmpSlice.Name())
			cl := gosseract.NewClient()
			_ = cl.SetLanguage("eng")
			_ = cl.SetWhitelist("0123456789RpIDRidri.,:()/- ")
			cl.SetImage(tmpSlice.Name())
			if t, er := cl.Text(); er == nil {
				variants = append(variants, normalizeOCRText(t))
			}
			cl.Close()
			cl2 := gosseract.NewClient()
			_ = cl2.SetLanguage("eng")
			_ = cl2.SetWhitelist("0123456789., ")
			cl2.SetImage(tmpSlice.Name())
			if td, er2 := cl2.Text(); er2 == nil {
				variants = append(variants, normalizeOCRText(td))
			}
			cl2.Close()
			_ = os.Remove(tmpSlice.Name())
		}
	}

	aggregate := strings.Join(variants, " ")
	out["aggregate"] = aggregate
	log.Printf("OCR passes summary base=%d totalVariants=%d length=%d", 5, len(variants), len(aggregate))
	return out, nil
}
