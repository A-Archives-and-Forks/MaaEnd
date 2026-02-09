package automaticcharactertutorial

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
)

// Helper: Binarize image for Template (White Icon, Black Background)
// Threshold: 100 (out of 255)
func binarizeTemplateTest(src image.Image) image.Image {
	bounds := src.Bounds()
	dst := image.NewRGBA(bounds)
	threshold := uint32(25700) // 100/255 * 65535 ≈ 25700
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := src.At(x, y).RGBA()
			if r > threshold && g > threshold && b > threshold {
				dst.Set(x, y, color.White)
			} else {
				dst.Set(x, y, color.Black)
			}
		}
	}
	return dst
}

// Helper: Binarize image for Search (White Icon, Black Background)
func binarizeSearchTest(src image.Image) image.Image {
	bounds := src.Bounds()
	dst := image.NewRGBA(bounds)
	threshold := uint32(25700) // 100/255 * 65535 ≈ 25700
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := src.At(x, y).RGBA()
			if r > threshold && g > threshold && b > threshold {
				dst.Set(x, y, color.White)
			} else {
				dst.Set(x, y, color.Black)
			}
		}
	}
	return dst
}

// Helper: Save image to file
func saveImgTest(t *testing.T, name string, img image.Image) {
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

// TestRecognitionDebug verifies the ROI cropping for Skill Match (recognition.go).
// Usage: Save a failing screenshot as "test_skill.png" in this directory and run `go test -v -run TestRecognitionDebug`
func TestRecognitionDebug(t *testing.T) {
	// 1. Load test image
	imgPath := "test_skill.png"
	f, err := os.Open(imgPath)
	if os.IsNotExist(err) {
		t.Skipf("Skipping test: %s not found. Please save a screenshot of the game as 'test_skill.png' in this directory to debug.", imgPath)
	}
	if err != nil {
		t.Fatalf("Failed to open image: %v", err)
	}
	defer f.Close()

	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("Failed to decode image: %v", err)
	}

	// 2. Define Parameters (Must match recognition.go)
	topROI := []int{617, 49, 45, 66}
	skillROI := []int{629, 60, 22, 22} // Updated center crop
	bottomROIs := [][]int{
		{1224, 623, 40, 40},
		{1160, 623, 40, 40},
		{1097, 624, 40, 40},
		{1034, 624, 40, 40},
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

	// 3. Process and Save Template (SkillROI)
	t.Log("Processing Skill ROI (Template)...")
	skillRect := image.Rect(skillROI[0], skillROI[1], skillROI[0]+skillROI[2], skillROI[1]+skillROI[3])
	skillCrop := subImager.SubImage(skillRect)
	saveImgTest(t, "debug_skill_template_original.png", skillCrop)

	// Apply Binarization (Template Style)
	binarizedTemplate := binarizeTemplateTest(skillCrop)
	saveImgTest(t, "debug_skill_template_binarized.png", binarizedTemplate)

	// Optional: TopROI for reference
	topRect := image.Rect(topROI[0], topROI[1], topROI[0]+topROI[2], topROI[1]+topROI[3])
	topCrop := subImager.SubImage(topRect)
	saveImgTest(t, "debug_skill_top_reference.png", topCrop)

	// 4. Process and Save Search Image (Whole Image Binarized)
	t.Log("Processing Search Image...")
	searchImg := binarizeSearchTest(img)
	saveImgTest(t, "debug_skill_search_image.png", searchImg)

	// 5. Save Bottom ROIs from the Search Image
	t.Log("Processing Bottom ROIs (from Search Image)...")
	searchSubImager, _ := searchImg.(SubImager)
	for i, roi := range bottomROIs {
		r := image.Rect(roi[0], roi[1], roi[0]+roi[2], roi[1]+roi[3])
		crop := searchSubImager.SubImage(r)
		saveImgTest(t, fmt.Sprintf("debug_skill_bottom_roi_%d_binarized.png", i), crop)
	}

	// 6. Generate ROI Map (Draw boxes on original image)
	bounds := img.Bounds()
	debugMap := image.NewRGBA(bounds)
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			debugMap.Set(x, y, img.At(x, y))
		}
	}

	drawBox := func(roi []int, c color.RGBA) {
		x, y, w, h := roi[0], roi[1], roi[2], roi[3]
		for i := 0; i < w; i++ {
			debugMap.Set(x+i, y, c)
			debugMap.Set(x+i, y+h-1, c)
		}
		for i := 0; i < h; i++ {
			debugMap.Set(x, y+i, c)
			debugMap.Set(x+w-1, y+i, c)
		}
	}

	drawBox(topROI, color.RGBA{0, 0, 255, 255})
	drawBox(skillROI, color.RGBA{0, 255, 255, 255})
	for _, roi := range bottomROIs {
		drawBox(roi, color.RGBA{255, 0, 0, 255})
	}
	for _, roi := range keyROIs {
		drawBox(roi, color.RGBA{0, 255, 0, 255})
	}

	saveImgTest(t, "debug_skill_map.png", debugMap)

	t.Log("Test complete. Please check the generated 'debug_skill_*.png' files.")
}
