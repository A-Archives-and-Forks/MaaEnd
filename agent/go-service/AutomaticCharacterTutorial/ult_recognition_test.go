package automaticcharactertutorial

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
)

// Helper: Resize function (Copied from ult_recognition.go for testing)
func resizeImgTest(src image.Image, newW, newH int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	bounds := src.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	xRatio := float64(srcW) / float64(newW)
	yRatio := float64(srcH) / float64(newH)

	for y := 0; y < newH; y++ {
		for x := 0; x < newW; x++ {
			var r, g, b, a, count uint64
			srcStartX := int(float64(x) * xRatio)
			srcStartY := int(float64(y) * yRatio)
			srcEndX := int(float64(x+1) * xRatio)
			srcEndY := int(float64(y+1) * yRatio)

			if srcEndX > srcW {
				srcEndX = srcW
			}
			if srcEndY > srcH {
				srcEndY = srcH
			}
			if srcEndX <= srcStartX {
				srcEndX = srcStartX + 1
			}
			if srcEndY <= srcStartY {
				srcEndY = srcStartY + 1
			}

			for sy := srcStartY; sy < srcEndY; sy++ {
				for sx := srcStartX; sx < srcEndX; sx++ {
					pr, pg, pb, pa := src.At(bounds.Min.X+sx, bounds.Min.Y+sy).RGBA()
					r += uint64(pr)
					g += uint64(pg)
					b += uint64(pb)
					a += uint64(pa)
					count++
				}
			}

			if count > 0 {
				dst.Set(x, y, color.RGBA64{
					R: uint16(r / count),
					G: uint16(g / count),
					B: uint16(b / count),
					A: uint16(a / count),
				})
			}
		}
	}
	return dst
}

// Helper: Save image to file
func saveImg(t *testing.T, name string, img image.Image) {
	f, err := os.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	t.Logf("Saved debug image: %s", name)
}

// TestUltRecognitionDebug verifies the ROI cropping and resizing logic.
// Usage: Save a failing screenshot as "test.png" in this directory and run `go test -v -run TestUltRecognitionDebug`
func TestUltRecognitionDebug(t *testing.T) {
	// 1. Load test image
	imgPath := "test.png"
	f, err := os.Open(imgPath)
	if os.IsNotExist(err) {
		t.Skipf("Skipping test: %s not found. Please save a screenshot of the game as 'test.png' in this directory to debug.", imgPath)
	}
	if err != nil {
		t.Fatalf("Failed to open image: %v", err)
	}
	defer f.Close()

	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("Failed to decode image: %v", err)
	}

	// 2. Define Parameters (Must match ult_recognition.go)
	topROI := []int{617, 49, 45, 66}
	skillROI := []int{626, 57, 28, 28} // Add SkillROI
	ultROIs := [][]int{
		{1221, 574, 40, 40},
		{1159, 574, 40, 40},
		{1097, 574, 40, 40},
		{1035, 574, 40, 40},
	}
	keyROIs := [][]int{
		{1233, 670, 20, 20},
		{1169, 670, 20, 20},
		{1105, 670, 20, 20},
		{1041, 670, 21, 20},
	}

	type SubImager interface {
		SubImage(r image.Rectangle) image.Image
	}
	subImager, ok := img.(SubImager)
	if !ok {
		t.Fatal("Image does not support SubImage")
	}

	// 3. Process and Save Template (Using SkillROI)
	t.Log("Processing Skill ROI (Template)...")
	skillRect := image.Rect(skillROI[0], skillROI[1], skillROI[0]+skillROI[2], skillROI[1]+skillROI[3])
	skillCrop := subImager.SubImage(skillRect)
	saveImg(t, "debug_ult_template_original.png", skillCrop)

	// Resize logic (20x20) - Matching the logic in ult_recognition.go
	resizedTemplate := resizeImgTest(skillCrop, 20, 20)
	saveImg(t, "debug_ult_template_resized.png", resizedTemplate)

	// Optional: Still save TopROI for reference
	topRect := image.Rect(topROI[0], topROI[1], topROI[0]+topROI[2], topROI[1]+topROI[3])
	topCrop := subImager.SubImage(topRect)
	saveImg(t, "debug_top_roi_reference.png", topCrop)

	// 4. Process and Save Bottom ROIs
	t.Log("Processing Bottom Ult ROIs...")
	for i, roi := range ultROIs {
		r := image.Rect(roi[0], roi[1], roi[0]+roi[2], roi[1]+roi[3])
		crop := subImager.SubImage(r)
		saveImg(t, fmt.Sprintf("debug_ult_roi_%d.png", i), crop)
	}

	// 5. Process and Save Key ROIs
	t.Log("Processing Key ROIs...")
	for i, roi := range keyROIs {
		r := image.Rect(roi[0], roi[1], roi[0]+roi[2], roi[1]+roi[3])
		crop := subImager.SubImage(r)
		saveImg(t, fmt.Sprintf("debug_key_roi_%d.png", i), crop)
	}

	// 6. Generate ROI Map (Draw boxes on original image)
	// We convert to RGBA to draw on it
	bounds := img.Bounds()
	debugMap := image.NewRGBA(bounds)
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			debugMap.Set(x, y, img.At(x, y))
		}
	}

	// Simple draw box function
	drawBox := func(roi []int, c color.RGBA) {
		x, y, w, h := roi[0], roi[1], roi[2], roi[3]
		// Top & Bottom
		for i := 0; i < w; i++ {
			debugMap.Set(x+i, y, c)
			debugMap.Set(x+i, y+h-1, c)
		}
		// Left & Right
		for i := 0; i < h; i++ {
			debugMap.Set(x, y+i, c)
			debugMap.Set(x+w-1, y+i, c)
		}
	}

	// Draw Top (Blue)
	drawBox(topROI, color.RGBA{0, 0, 255, 255})
	// Draw Skill (Cyan)
	drawBox(skillROI, color.RGBA{0, 255, 255, 255})

	// Draw Ult (Red)
	for _, roi := range ultROIs {
		drawBox(roi, color.RGBA{255, 0, 0, 255})
	}

	// Draw Key (Green)
	for _, roi := range keyROIs {
		drawBox(roi, color.RGBA{0, 255, 0, 255})
	}

	saveImg(t, "debug_ult_map.png", debugMap)

	t.Log("Test complete. Please check the generated 'debug_*.png' files to verify ROIs and Template quality.")
}
