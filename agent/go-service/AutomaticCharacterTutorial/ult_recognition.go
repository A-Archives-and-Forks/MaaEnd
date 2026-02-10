package automaticcharactertutorial

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"os"
	"strconv"
	"time"

	"github.com/MaaXYZ/maa-framework-go/v3"
	"github.com/rs/zerolog/log"
) // UltimateSkillRecognition detects which character has an ultimate ready
// UltimateSkillRecognition 检测哪个角色的终结技已就绪
// 终结技识别：检测顶部提示图标是否与下方终结技图标匹配，并识别对应按键
type UltimateSkillRecognition struct{}

// UltRecognitionParams defines the structure for custom parameters
// UltRecognitionParams 定义自定义参数的结构
type UltRecognitionParams struct {
	TopROI      []int   `json:"top_roi"`
	SkillROI    []int   `json:"skill_roi"`
	UltROIs     [][]int `json:"ult_rois"`
	KeyROIs     [][]int `json:"key_rois"`
	Threshold   float64 `json:"threshold"`
	Recognition string  `json:"recognition"`
	Method      int     `json:"method"`
}

// Run implements the custom recognition logic
// Run 实现自定义识别逻辑
func (r *UltimateSkillRecognition) Run(ctx *maa.Context, arg *maa.CustomRecognitionArg) (*maa.CustomRecognitionResult, bool) {
	// 1. Parse Parameters from Pipeline JSON
	// 1. 解析流水线 JSON 中的参数
	var params UltRecognitionParams
	if err := json.Unmarshal([]byte(arg.CustomRecognitionParam), &params); err != nil {
		log.Error().Err(err).Msg("Failed to parse custom recognition params")
		return nil, false
	}

	// Validate essential parameters
	// 验证必要参数
	if len(params.SkillROI) < 4 || len(params.UltROIs) == 0 {
		log.Error().Msg("Invalid recognition parameters: missing ROI definitions")
		return nil, false
	}

	// Default threshold if not set
	// 设置默认阈值
	if params.Threshold == 0 {
		params.Threshold = 0.1
	}
	// Normalize threshold if > 1.0
	// 归一化阈值
	if params.Threshold > 1.0 {
		params.Threshold = params.Threshold / 100.0
	}

	// Default recognition method if not set
	// 设置默认识别方法
	if params.Recognition == "" {
		params.Recognition = "TemplateMatch"
	}
	if params.Method == 0 {
		params.Method = 5 // TM_CCOEFF_NORMED
	}

	img := arg.Img
	if img == nil {
		return nil, false
	}

	// Helper interface for cropping
	// 辅助接口：裁剪
	type SubImager interface {
		SubImage(r image.Rectangle) image.Image
	}
	subImager, ok := img.(SubImager)
	if !ok {
		log.Error().Msg("Image does not support SubImage")
		return nil, false
	}

	// Simple Box-Sampling Resize function (Better for downscaling)
	// 简单的盒采样缩放函数（更适合缩小）
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
				// 计算源矩形内的平均像素值
				var r, g, b, a, count uint64
				srcStartX := int(float64(x) * xRatio)
				srcStartY := int(float64(y) * yRatio)
				srcEndX := int(float64(x+1) * xRatio)
				srcEndY := int(float64(y+1) * yRatio)

				// Clamp
				// 限制范围
				if srcEndX > srcW {
					srcEndX = srcW
				}
				if srcEndY > srcH {
					srcEndY = srcH
				}

				// If ratios are small (upscaling or small downscaling), ensure at least one pixel is read
				// 如果比例很小（放大或轻微缩小），确保至少读取一个像素
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
	// 2. 从 SkillROI 准备模板（比 TopROI 更精确）
	// Function to crop, RESIZE and save template to a file in resource/image directory
	// 裁剪、缩放并保存模板到 resource/image 目录的函数
	createTempTemplate := func(roi []int) (string, error) {
		if len(roi) < 4 {
			return "", os.ErrInvalid
		}
		rect := image.Rect(roi[0], roi[1], roi[0]+roi[2], roi[1]+roi[3])
		if !rect.In(img.Bounds()) {
			rect = rect.Intersect(img.Bounds())
		}
		if rect.Empty() {
			return "", os.ErrInvalid
		}

		cropImg := subImager.SubImage(rect)

		// RESIZE: Scale down the 28x28 skill icon.
		// Previous attempt 20x20 might be too small.
		// Let's try 24x24 as well to be consistent with recognition.go
		// 缩放：缩小 28x28 的技能图标。
		// 之前的尝试 20x20 可能太小了。
		// 让我们尝试 24x24，以便与 recognition.go 保持一致。
		resizedImg := resizeImg(cropImg, 24, 24)

		// Use unique key for in-memory image
		templateKey := fmt.Sprintf("UltimateMatchTemplate_%d", time.Now().UnixNano())
		// Note: In current SDK version, OverrideImage returns bool, not error.
		// Adapting to current SDK while implementing the logic.
		if !ctx.OverrideImage(templateKey, resizedImg) {
			return "", fmt.Errorf("failed to override image")
		}
		return templateKey, nil
	}

	// Try to get template from SkillROI (Primary) or TopROI (Fallback)
	// 尝试从 SkillROI 获取模板（主要），或者从 TopROI 获取（回退）
	log.Debug().Ints("SkillROI", params.SkillROI).Msg("Capturing ultimate template from SkillROI")
	templatePath, err := createTempTemplate(params.SkillROI)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to create ultimate template from SkillROI, trying TopROI")
		// Only try TopROI if it is valid
		if len(params.TopROI) >= 4 {
			templatePath, err = createTempTemplate(params.TopROI)
		}
		if err != nil {
			log.Error().Err(err).Msg("Failed to create ultimate template")
			return nil, false
		}
	}

	// 3. Match Template against UltROIs
	// 3. 匹配模板与 UltROIs
	bestIdx := -1
	maxScore := -1.0

	for i, ultROI := range params.UltROIs {
		if len(ultROI) < 4 {
			continue
		}

		taskName := "UltMatch_" + strconv.Itoa(i)
		tmParam := map[string]any{
			taskName: map[string]any{
				"recognition": params.Recognition,
				"template":    templatePath,
				"threshold":   params.Threshold,
				"roi":         ultROI,
				"method":      params.Method,
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
	// 检查最佳匹配是否足够好
	if maxScore < params.Threshold {
		log.Debug().Float64("maxScore", maxScore).Msg("No matching ultimate icon found")
		return nil, false
	}

	log.Info().Int("bestIdx", bestIdx).Float64("score", maxScore).Msg("Ultimate matched")

	// 4. Identify Key Number using Template Match (1.png - 4.png)
	// 4. 使用模板匹配识别按键数字（1.png - 4.png）
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
	// 5. 返回结果
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
