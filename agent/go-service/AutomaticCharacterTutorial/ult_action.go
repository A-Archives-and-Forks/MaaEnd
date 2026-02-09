package automaticcharactertutorial

import (
	"encoding/json"
	"time"

	"github.com/MaaXYZ/maa-framework-go/v3"
	"github.com/rs/zerolog/log"
)

// UltimateSkillAction implements logic to long press the ultimate key
// 终结技动作：根据识别到的按键数字长按对应的键盘按键
type UltimateSkillAction struct{}

// Run implements the custom action logic
func (a *UltimateSkillAction) Run(ctx *maa.Context, arg *maa.CustomActionArg) bool {
	detailStr := arg.RecognitionDetail.DetailJson

	// Define a unified structure to handle both flat and nested JSON
	type ActionDetail struct {
		Index  int `json:"index"`
		KeyNum int `json:"key_num"`
	}

	var targetDetail ActionDetail
	var parsed bool

	// 1. Try to parse as nested structure (standard Framework result)
	var nested struct {
		Best struct {
			Detail ActionDetail `json:"detail"`
		} `json:"best"`
	}
	if err := json.Unmarshal([]byte(detailStr), &nested); err == nil && nested.Best.Detail.KeyNum != 0 {
		targetDetail = nested.Best.Detail
		parsed = true
	}

	// 2. If nested parsing failed or empty, try flat structure (custom result)
	if !parsed {
		if err := json.Unmarshal([]byte(detailStr), &targetDetail); err == nil {
			parsed = true
		}
	}

	if !parsed {
		log.Error().Str("detail", detailStr).Msg("Failed to parse ult action detail")
		return false
	}

	if targetDetail.KeyNum >= 1 && targetDetail.KeyNum <= 4 {
		keyCode := 48 + targetDetail.KeyNum
		log.Info().Int("keyNum", targetDetail.KeyNum).Int("keyCode", keyCode).Msg("Long pressing ultimate skill key (0.3s)")

		ctrl := ctx.GetTasker().GetController()

		// Press Down
		ctrl.PostKeyDown(int32(keyCode)).Wait()

		// Hold for 300ms
		time.Sleep(300 * time.Millisecond)

		// Release
		ctrl.PostKeyUp(int32(keyCode)).Wait()

		return true
	}

	log.Warn().Int("index", targetDetail.Index).Int("keyNum", targetDetail.KeyNum).Msg("No valid key number for ult, skipping action")
	return false
}
