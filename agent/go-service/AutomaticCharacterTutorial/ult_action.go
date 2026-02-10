package automaticcharactertutorial

import (
	"encoding/json"

	"github.com/MaaXYZ/maa-framework-go/v3"
	"github.com/rs/zerolog/log"
)

// UltimateSkillAction implements logic to long press the ultimate key
// UltimateSkillAction 实现长按终结技按键的逻辑
// 终结技动作：根据识别到的按键数字长按对应的键盘按键
type UltimateSkillAction struct{}

// Run implements the custom action logic
// Run 实现自定义动作逻辑
func (a *UltimateSkillAction) Run(ctx *maa.Context, arg *maa.CustomActionArg) bool {
	// 1. Parse Detail from Recognition
	// 1. 解析识别结果详情
	// Since we control the Custom Recognition result, it is a flat JSON.
	// 由于我们控制自定义识别结果，它是扁平的 JSON。
	detailStr := arg.RecognitionDetail.DetailJson
	var detail struct {
		KeyNum int `json:"key_num"`
	}

	if err := json.Unmarshal([]byte(detailStr), &detail); err != nil {
		log.Error().Err(err).Str("detail", detailStr).Msg("Failed to parse recognition detail")
		return false
	}

	// 2. Click the key using standard LongPressKey Action
	// 2. 使用标准长按动作点击按键
	if detail.KeyNum >= 1 && detail.KeyNum <= 4 {
		keyCode := 48 + detail.KeyNum
		log.Info().Int("keyNum", detail.KeyNum).Msg("Long pressing ultimate skill key")

		// Use built-in LongPressKey action
		// 使用内置的 LongPressKey 动作
		param := map[string]interface{}{
			"key":      keyCode,
			"duration": 300, // 300ms
		}
		paramBytes, _ := json.Marshal(param)
		ctx.RunAction("LongPressKey", arg.RecognitionDetail.Box, string(paramBytes))
		return true
	}

	log.Warn().Int("keyNum", detail.KeyNum).Msg("No valid key number for ult, skipping action")
	return false
}
