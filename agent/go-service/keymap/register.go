package keymap

import maa "github.com/MaaXYZ/maa-framework-go/v4"

func Register() {
	maa.AgentServerRegisterCustomAction("_ClickKey", &_ClickKey{})
	maa.AgentServerRegisterCustomAction("_LongPressKey", &_LongPressKey{})
	maa.AgentServerRegisterCustomAction("_KeyDown", &_KeyDown{})
	maa.AgentServerRegisterCustomAction("_KeyUp", &_KeyUp{})
}
