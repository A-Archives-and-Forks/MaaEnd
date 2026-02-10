package automaticcharactertutorial

import (
	"encoding/json"

	"github.com/MaaXYZ/maa-framework-go/v3"
	"github.com/rs/zerolog/log"
)

// DynamicMatchAction presses the key identified by Recognition
// DynamicMatchAction 实现点击由识别模块识别出的按键的逻辑
type DynamicMatchAction struct{}

// Run implements the custom action logic
// Run 实现自定义动作逻辑
func (a *DynamicMatchAction) Run(ctx *maa.Context, arg *maa.CustomActionArg) bool {
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

	// 2. Click the key using standard Action
	// 2. 使用标准动作点击按键
	if detail.KeyNum >= 1 && detail.KeyNum <= 4 {
		keyCode := 48 + detail.KeyNum
		log.Info().Int("keyNum", detail.KeyNum).Msg("Pressing skill key")

		// Use built-in ClickKey action
		// 使用内置的 ClickKey 动作
		param := map[string]interface{}{
			"key": keyCode,
		}
		paramBytes, _ := json.Marshal(param)
		ctx.RunAction("ClickKey", arg.RecognitionDetail.Box, string(paramBytes))
		return true
	}

	log.Warn().Int("keyNum", detail.KeyNum).Msg("No valid key number recognized, skipping action")
	return false
}
