package keymap

import maa "github.com/MaaXYZ/maa-framework-go/v4"

func Register() {
	maa.AgentServerRegisterCustomAction("km:ClickKey", &_ClickKey{})
	maa.AgentServerRegisterCustomAction("km:LongPressKey", &_LongPressKey{})
	maa.AgentServerRegisterCustomAction("km:KeyDown", &_KeyDown{})
	maa.AgentServerRegisterCustomAction("km:KeyUp", &_KeyUp{})
}
