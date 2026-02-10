package automaticcharactertutorial

import "github.com/MaaXYZ/maa-framework-go/v3"

// Register registers the custom recognition and action
// 注册自定义识别和动作
func Register() {
	maa.AgentServerRegisterCustomRecognition("DynamicMatchRecognition", &DynamicMatchRecognition{})

	maa.AgentServerRegisterCustomAction("DynamicMatchAction", &DynamicMatchAction{})

	maa.AgentServerRegisterCustomRecognition("UltimateSkillRecognition", &UltimateSkillRecognition{})

	maa.AgentServerRegisterCustomAction("UltimateSkillAction", &UltimateSkillAction{})
}
