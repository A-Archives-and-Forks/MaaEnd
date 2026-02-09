package automaticcharactertutorial

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/MaaXYZ/maa-framework-go/v3"
	"github.com/rs/zerolog/log"
)

// UltimateSkillRecognition detects which character has an ultimate ready
// 终结技识别：检测顶部提示图标是否与下方终结技图标匹配，并识别对应按键
type UltimateSkillRecognition struct{}

// UltRecognitionParams defines the structure for custom parameters
type UltRecognitionParams struct {
	TopROI    []int   `json:"top_roi"`
	SkillROI  []int   `json:"skill_roi"`
	UltROIs   [][]int `json:"ult_rois"`
	KeyROIs   [][]int `json:"key_rois"`
	Threshold float64 `json:"threshold"`
}

// Run implements the custom recognition logic
func (r *UltimateSkillRecognition) Run(ctx *maa.Context, arg *maa.CustomRecognitionArg) (*maa.CustomRecognitionResult, bool) {
	// 1. Parse Parameters from Pipeline JSON
	var params UltRecognitionParams
	if err := json.Unmarshal([]byte(arg.CustomRecognitionParam), &params); err != nil {
		log.Error().Err(err).Msg("Failed to parse custom recognition params")
		return nil, false
	}

	// Validate essential parameters
	if len(params.SkillROI) < 4 || len(params.UltROIs) == 0 {
		log.Error().Msg("Invalid recognition parameters: missing ROI definitions")
		return nil, false
	}

	// Default threshold if not set
	if params.Threshold == 0 {
		params.Threshold = 0.1
	}
	// Normalize threshold if > 1.0
	if params.Threshold > 1.0 {
		params.Threshold = params.Threshold / 100.0
	}

	img := arg.Img
	if img == nil {
		return nil, false
	}

	// Helper interface for cropping
	type SubImager interface {
		SubImage(r image.Rectangle) image.Image
	}
	subImager, ok := img.(SubImager)
	if !ok {
		log.Error().Msg("Image does not support SubImage")
		return nil, false
	}

	// Simple Box-Sampling Resize function (Better for downscaling)
	resizeImg := func(src image.Image, newW, newH int) image.Image {
		dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
		bounds := src.Bounds()
		srcW := bounds.Dx()
		srcH := bounds.Dy()

		xRatio := float64(srcW) / float64(newW)
		yRatio := float64(srcH) / float64(newH)

		for y := 0; y < newH; y++ {
			for x := 0; x < newW; x++ {
				// Average pixel values in the source rectangle
				var r, g, b, a, count uint64
				srcStartX := int(float64(x) * xRatio)
				srcStartY := int(float64(y) * yRatio)
				srcEndX := int(float64(x+1) * xRatio)
				srcEndY := int(float64(y+1) * yRatio)

				// Clamp
				if srcEndX > srcW {
					srcEndX = srcW
				}
				if srcEndY > srcH {
					srcEndY = srcH
				}

				// If ratios are small (upscaling or small downscaling), ensure at least one pixel is read
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

	// 2. Prepare Template from SkillROI (More precise than TopROI)
	// Function to crop, RESIZE and save template to a file in resource/image directory
	createTempTemplate := func(roi []int) (string, string, error) {
		if len(roi) < 4 {
			return "", "", os.ErrInvalid
		}
		rect := image.Rect(roi[0], roi[1], roi[0]+roi[2], roi[1]+roi[3])
		if !rect.In(img.Bounds()) {
			rect = rect.Intersect(img.Bounds())
		}
		if rect.Empty() {
			return "", "", os.ErrInvalid
		}

		cropImg := subImager.SubImage(rect)

		// RESIZE: Scale down the 28x28 skill icon.
		// Previous attempt 20x20 might be too small.
		// Let's try 24x24 as well to be consistent with recognition.go
		resizedImg := resizeImg(cropImg, 24, 24)

		// Use a relative path within resource/image
		relDir := "AutomaticCharacterTutorial"
		// Use unique filename to avoid caching issues
		fileName := fmt.Sprintf("ultimate_template_%d.png", time.Now().UnixNano())
		relPath := relDir + "/" + fileName

		// Full path for writing the file
		absDir := filepath.Join("resource", "image", relDir)
		if err := os.MkdirAll(absDir, 0755); err != nil {
			return "", "", err
		}

		fullPath := filepath.Join(absDir, fileName)
		f, err := os.Create(fullPath)
		if err != nil {
			return "", "", err
		}
		defer f.Close()

		if err := png.Encode(f, resizedImg); err != nil {
			return "", "", err
		}

		return relPath, fullPath, nil
	}

	// Try to get template from SkillROI (Primary) or TopROI (Fallback)
	log.Debug().Ints("SkillROI", params.SkillROI).Msg("Capturing ultimate template from SkillROI")
	templatePath, fullTemplatePath, err := createTempTemplate(params.SkillROI)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to create ultimate template from SkillROI, trying TopROI")
		// Only try TopROI if it is valid
		if len(params.TopROI) >= 4 {
			templatePath, fullTemplatePath, err = createTempTemplate(params.TopROI)
		}
		if err != nil {
			log.Error().Err(err).Msg("Failed to create ultimate template")
			return nil, false
		}
	}
	// Clean up the template file after recognition is done
	defer os.Remove(fullTemplatePath)

	// 3. Match Template against UltROIs
	bestIdx := -1
	maxScore := -1.0

	for i, ultROI := range params.UltROIs {
		if len(ultROI) < 4 {
			continue
		}

		taskName := "UltMatch_" + strconv.Itoa(i)
		tmParam := map[string]any{
			taskName: map[string]any{
				"recognition": "TemplateMatch",
				"template":    templatePath,
				"threshold":   params.Threshold,
				"roi":         ultROI,
				"method":      5, // TM_CCOEFF_NORMED
			},
		}

		res := ctx.RunRecognition(taskName, img, tmParam)

		var score float64
		if res != nil {
			var detail struct {
				All []struct {
					Score float64 `json:"score"`
				} `json:"all"`
				Best *struct {
					Score float64 `json:"score"`
				} `json:"best"`
			}

			if err := json.Unmarshal([]byte(res.DetailJson), &detail); err == nil {
				// 优先读取 Best
				if detail.Best != nil {
					score = detail.Best.Score
				} else if len(detail.All) > 0 {
					// 如果 Best 为空 (Hit=false)，尝试从 All 中找最大值
					for _, item := range detail.All {
						if item.Score > score {
							score = item.Score
						}
					}
				}
			}
		}

		log.Debug().Int("index", i).Float64("score", score).Msg("Ult template match result")

		if score > maxScore {
			maxScore = score
			bestIdx = i
		}
	}

	// Check if the best match is good enough
	if maxScore < params.Threshold {
		log.Info().Float64("maxScore", maxScore).Msg("No matching ultimate icon found")
		return nil, false
	}

	log.Info().Int("bestIdx", bestIdx).Float64("score", maxScore).Msg("Ultimate matched")

	// 4. Identify Key Number using Template Match (1.png - 4.png)
	keyNum := -1
	if bestIdx >= 0 && bestIdx < len(params.KeyROIs) {
		baseKeyROI := params.KeyROIs[bestIdx]
		searchROI := []int{
			baseKeyROI[0] - 10,
			baseKeyROI[1] - 10,
			baseKeyROI[2] + 20,
			baseKeyROI[3] + 20,
		}

		bestKeyScore := -1.0

		for k := 1; k <= 4; k++ {
			templateName := "AutomaticCharacterTutorial/" + strconv.Itoa(k) + ".png"
			taskName := "UltMatchKey_" + strconv.Itoa(k)

			tmParam := map[string]any{
				taskName: map[string]any{
					"recognition": "TemplateMatch",
					"template":    templateName,
					"threshold":   0.6,
					"roi":         searchROI,
					"method":      5,
				},
			}

			res := ctx.RunRecognition(taskName, img, tmParam)
			if res != nil && res.Hit {
				var detail struct {
					Best struct {
						Score float64 `json:"score"`
					} `json:"best"`
				}
				score := 0.0
				if err := json.Unmarshal([]byte(res.DetailJson), &detail); err == nil {
					score = detail.Best.Score
				}

				log.Debug().Int("key", k).Float64("score", score).Msg("Ult Key number match result")

				if score > bestKeyScore {
					bestKeyScore = score
					keyNum = k
				}
			}
		}

		if bestKeyScore < 0.5 {
			log.Warn().Float64("score", bestKeyScore).Msg("Ult Key number match score too low")
		}
	}

	// 5. Return Result
	detailBytes, _ := json.Marshal(map[string]any{
		"index":   bestIdx,
		"score":   maxScore,
		"key_num": keyNum,
	})

	box := maa.Rect{}
	if bestIdx >= 0 && bestIdx < len(params.UltROIs) {
		r := params.UltROIs[bestIdx]
		if len(r) >= 4 {
			box = maa.Rect{r[0], r[1], r[2], r[3]}
		}
	}

	return &maa.CustomRecognitionResult{
		Box:    box,
		Detail: string(detailBytes),
	}, true
}
