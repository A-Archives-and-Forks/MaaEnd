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
) // DynamicMatchRecognition implements logic to match a skill icon and recognize the corresponding key
// DynamicMatchRecognition 实现匹配技能图标并识别对应按键的逻辑
type DynamicMatchRecognition struct{}

// RecognitionParams defines the structure for custom parameters
// RecognitionParams 定义自定义参数的结构
type RecognitionParams struct {
	TopROI     []int   `json:"top_roi"`
	SkillROI   []int   `json:"skill_roi"`
	BottomROIs [][]int `json:"bottom_rois"`
	KeyROIs    [][]int `json:"key_rois"`
	Threshold  float64 `json:"threshold"`
}

// Run implements the custom recognition logic
// Run 实现自定义识别逻辑
func (r *DynamicMatchRecognition) Run(ctx *maa.Context, arg *maa.CustomRecognitionArg) (*maa.CustomRecognitionResult, bool) {
	// 1. Parse Parameters from Pipeline JSON
	// 1. 解析流水线 JSON 中的参数
	var params RecognitionParams
	if err := json.Unmarshal([]byte(arg.CustomRecognitionParam), &params); err != nil {
		log.Error().Err(err).Msg("Failed to parse custom recognition params")
		return nil, false
	}

	// Validate essential parameters
	// 验证必要参数
	if len(params.SkillROI) < 4 || len(params.BottomROIs) == 0 {
		log.Error().Msg("Invalid recognition parameters: missing ROI definitions")
		return nil, false
	}

	// Default threshold if not set
	// 设置默认阈值
	if params.Threshold == 0 {
		params.Threshold = 0.7
	}
	// Normalize threshold if > 1.0
	// 归一化阈值
	if params.Threshold > 1.0 {
		params.Threshold = params.Threshold / 100.0
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

	// Helper: Binarize image (White Icon, Black Background)
	// 辅助函数：二值化图像（白色图标，黑色背景）
	binarize := func(src image.Image) image.Image {
		bounds := src.Bounds()
		dst := image.NewRGBA(bounds)
		thresh := uint32(25700) // 100/255 * 65535 ≈ 25700
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				r, g, b, _ := src.At(x, y).RGBA()
				if r > thresh && g > thresh && b > thresh {
					dst.Set(x, y, color.White)
				} else {
					dst.Set(x, y, color.Black)
				}
			}
		}
		return dst
	}

	// 2. Prepare Template (SkillROI first, then TopROI)
	// 2. 准备模板（优先 SkillROI，其次 TopROI）
	createTempTemplate := func(roi []int, halfHeight bool) (string, error) {
		if len(roi) < 4 {
			return "", os.ErrInvalid
		}

		h := roi[3]
		if halfHeight {
			h = h / 2
		}

		rect := image.Rect(roi[0], roi[1], roi[0]+roi[2], roi[1]+h)
		if !rect.In(img.Bounds()) {
			rect = rect.Intersect(img.Bounds())
		}
		if rect.Empty() {
			return "", os.ErrInvalid
		}

		cropImg := subImager.SubImage(rect)
		binarizedImg := binarize(cropImg)

		// Use unique key for in-memory image
		templateKey := fmt.Sprintf("DynamicMatchTemplate_%d_%v", time.Now().UnixNano(), halfHeight)
		// Note: In current SDK version, OverrideImage returns bool, not error.
		// Adapting to current SDK while implementing the logic.
		if !ctx.OverrideImage(templateKey, binarizedImg) {
			return "", fmt.Errorf("failed to override image")
		}
		return templateKey, nil
	}

	log.Debug().Ints("SkillROI", params.SkillROI).Msg("Capturing template from SkillROI")
	templatePath, err := createTempTemplate(params.SkillROI, false)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to create template from SkillROI, trying TopROI")
		// Only try TopROI if it is valid
		if len(params.TopROI) >= 4 {
			templatePath, err = createTempTemplate(params.TopROI, false)
		}
		if err != nil {
			log.Error().Err(err).Msg("Failed to create template from both SkillROI and TopROI")
			return nil, false
		}
	}

	// 3. Match Template against BottomROIs
	// 3. 匹配底部候选区域
	// Pre-process the search image to match the template style (White on Black)
	// 预处理搜索图像以匹配模板风格（黑底白字）
	searchImg := binarize(img)

	bestIdx := -1
	maxScore := -1.0

	for i, bottomROI := range params.BottomROIs {
		if len(bottomROI) < 4 {
			continue
		}

		taskName := "DynamicMatch_" + strconv.Itoa(i)
		tmParam := map[string]any{
			taskName: map[string]any{
				"recognition": "TemplateMatch",
				"template":    templatePath,
				"threshold":   params.Threshold,
				"roi":         bottomROI,
				"method":      5, // TM_CCOEFF_NORMED
			},
		}

		// Use the pre-processed search image (White on Black)
		// 使用预处理后的搜索图像（黑底白字）
		res := ctx.RunRecognition(taskName, searchImg, tmParam)

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
			// Best effort to parse score
			// 尽力解析分数
			if err := json.Unmarshal([]byte(res.DetailJson), &detail); err == nil {
				if detail.Best != nil {
					score = detail.Best.Score
				} else if len(detail.All) > 0 {
					// Fallback: find max score in 'all'
					// 回退：在 'all' 中找到最大分数
					for _, item := range detail.All {
						if item.Score > score {
							score = item.Score
						}
					}
				}
			}
		} else {
			// Not hit, score is effectively 0 or low
			// 未命中，分数实际上为 0 或很低
			score = 0.0
		}

		log.Debug().Int("index", i).Float64("score", score).Msg("Template match result")

		if score > maxScore {
			maxScore = score
			bestIdx = i
		}
	}

	// Check if the best match is good enough
	// 检查最佳匹配是否足够好
	if maxScore < params.Threshold {
		log.Info().Float64("maxScore", maxScore).Msg("No matching skill icon found (score too low), trying half-height template")

		// Fallback: Try half-height template
		// 回退逻辑：尝试使用半高模板
		halfTemplatePath, err := createTempTemplate(params.SkillROI, true)
		if err == nil {
			for i, bottomROI := range params.BottomROIs {
				if len(bottomROI) < 4 {
					continue
				}

				taskName := "DynamicMatch_Half_" + strconv.Itoa(i)
				tmParam := map[string]any{
					taskName: map[string]any{
						"recognition": "TemplateMatch",
						"template":    halfTemplatePath,
						"threshold":   params.Threshold,
						"roi":         bottomROI,
						"method":      5, // TM_CCOEFF_NORMED
					},
				}

				res := ctx.RunRecognition(taskName, searchImg, tmParam)

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
						if detail.Best != nil {
							score = detail.Best.Score
						} else if len(detail.All) > 0 {
							for _, item := range detail.All {
								if item.Score > score {
									score = item.Score
								}
							}
						}
					}
				}

				log.Debug().Int("index", i).Float64("score", score).Msg("Half-height template match result")

				if score > maxScore {
					maxScore = score
					bestIdx = i
				}
			}
		}
	}

	if maxScore < params.Threshold {
		log.Info().Float64("maxScore", maxScore).Msg("No matching skill icon found even with half-height template")
		return nil, false
	}

	log.Info().Int("bestIdx", bestIdx).Float64("score", maxScore).Msg("Skill matched")

	// 4. Identify Key Number using Template Match (1.png - 4.png)
	// 4. 使用模板匹配识别按键数字（1.png - 4.png）
	// Only perform OCR on the KeyROI corresponding to the matched BottomROI
	// 仅对匹配到的 BottomROI 对应的 KeyROI 进行识别
	keyNum := -1
	if bestIdx >= 0 && bestIdx < len(params.KeyROIs) {
		// Get the KeyROI for the matched position
		// 获取匹配位置的 KeyROI
		baseKeyROI := params.KeyROIs[bestIdx]

		// Expand ROI slightly to ensure template fits and allows for slight offset
		// 稍微扩展 ROI 以确保模板适合并允许轻微偏移
		searchROI := []int{
			baseKeyROI[0] - 10,
			baseKeyROI[1] - 10,
			baseKeyROI[2] + 20,
			baseKeyROI[3] + 20,
		}

		bestKeyScore := -1.0

		for k := 1; k <= 4; k++ {
			templateName := "AutomaticCharacterTutorial/" + strconv.Itoa(k) + ".png"
			taskName := "MatchKey_" + strconv.Itoa(k)

			tmParam := map[string]any{
				taskName: map[string]any{
					"recognition": "TemplateMatch",
					"template":    templateName,
					"threshold":   0.6, // Lower threshold for small icons
					"roi":         searchROI,
					"method":      5,
				},
			}

			res := ctx.RunRecognition(taskName, img, tmParam)
			if res != nil {
				var detail struct {
					Best *struct {
						Score float64 `json:"score"`
					} `json:"best"`
					All []struct {
						Score float64 `json:"score"`
					} `json:"all"`
				}

				score := 0.0
				if err := json.Unmarshal([]byte(res.DetailJson), &detail); err == nil {
					if detail.Best != nil {
						score = detail.Best.Score
					} else if len(detail.All) > 0 {
						for _, item := range detail.All {
							if item.Score > score {
								score = item.Score
							}
						}
					}
				}

				log.Debug().Int("key", k).Float64("score", score).Msg("Key number match result")

				if score > bestKeyScore {
					bestKeyScore = score
					keyNum = k
				}
			}
		}

		// If score is too low, maybe it's not a valid number?
		// But we should return the best guess if it's reasonable.
		// 如果分数太低，可能不是有效的数字？但如果合理，我们应该返回最佳猜测。
		if bestKeyScore < 0.5 {
			log.Warn().Float64("score", bestKeyScore).Msg("Key number match score too low")
			// keyNum = -1 // Optional: strict check
		}
	}

	// 5. Return Result
	// 5. 返回结果
	detailBytes, _ := json.Marshal(map[string]any{
		"index":   bestIdx,
		"score":   maxScore,
		"key_num": keyNum,
	})

	// Box can be the matched BottomROI for visualization
	// Box 可以是匹配到的 BottomROI，用于可视化
	box := maa.Rect{}
	if bestIdx >= 0 && bestIdx < len(params.BottomROIs) {
		r := params.BottomROIs[bestIdx]
		if len(r) >= 4 {
			box = maa.Rect{r[0], r[1], r[2], r[3]}
		}
	}

	return &maa.CustomRecognitionResult{
		Box:    box,
		Detail: string(detailBytes),
	}, true
}
