package godd

// VirtualKey represents a custom virtual key code defined by the DD.dll driver.
// These codes map to specific keyboard keys and are not standard Windows VK_* values.
type VirtualKey int

// VK (Virtual Key)
const (
	// Function Keys
	VK_F1  VirtualKey = 101 // F1 function key
	VK_F2  VirtualKey = 102 // F2 function key
	VK_F3  VirtualKey = 103
	VK_F4  VirtualKey = 104
	VK_F5  VirtualKey = 105
	VK_F6  VirtualKey = 106
	VK_F7  VirtualKey = 107
	VK_F8  VirtualKey = 108
	VK_F9  VirtualKey = 109
	VK_F10 VirtualKey = 110
	VK_F11 VirtualKey = 111
	VK_F12 VirtualKey = 112

	// Special Keys
	VK_ESC     VirtualKey = 100 // ESC key
	VK_TAB     VirtualKey = 300 // Tab key
	VK_CAPS    VirtualKey = 400 // Caps Lock key
	VK_LSHIFT  VirtualKey = 500 // Left Shift key
	VK_RSHIFT  VirtualKey = 511 // Right Shift key
	VK_LCTRL   VirtualKey = 600
	VK_RCTRL   VirtualKey = 607
	VK_LWIN    VirtualKey = 601
	VK_RWIN    VirtualKey = 605
	VK_LALT    VirtualKey = 602
	VK_RALT    VirtualKey = 604
	VK_SPACE   VirtualKey = 603
	VK_ENTER   VirtualKey = 313
	VK_BKSPACE VirtualKey = 214 // Backspace
	VK_DELETE  VirtualKey = 706
	VK_HOME    VirtualKey = 704
	VK_END     VirtualKey = 707
	VK_PGUP    VirtualKey = 705
	VK_PGDN    VirtualKey = 708
	VK_INSERT  VirtualKey = 703
	VK_PAUSE   VirtualKey = 702
	VK_SCROLL  VirtualKey = 701 // ScrollWheel Lock
	VK_NUMLOCK VirtualKey = 810
	VK_PRINT   VirtualKey = 700 // Print

	// Arrow Keys
	VK_UP    VirtualKey = 709
	VK_DOWN  VirtualKey = 711
	VK_LEFT  VirtualKey = 710
	VK_RIGHT VirtualKey = 712

	// Numbers Row
	VK_1 VirtualKey = 201
	VK_2 VirtualKey = 202
	VK_3 VirtualKey = 203
	VK_4 VirtualKey = 204
	VK_5 VirtualKey = 205
	VK_6 VirtualKey = 206
	VK_7 VirtualKey = 207
	VK_8 VirtualKey = 208
	VK_9 VirtualKey = 209
	VK_0 VirtualKey = 210

	// Letters
	VK_A VirtualKey = 401 // A key
	VK_B VirtualKey = 505 // B key
	VK_C VirtualKey = 503
	VK_D VirtualKey = 403
	VK_E VirtualKey = 303
	VK_F VirtualKey = 404
	VK_G VirtualKey = 405
	VK_H VirtualKey = 406
	VK_I VirtualKey = 308
	VK_J VirtualKey = 407
	VK_K VirtualKey = 408
	VK_L VirtualKey = 409
	VK_M VirtualKey = 507
	VK_N VirtualKey = 506
	VK_O VirtualKey = 309
	VK_P VirtualKey = 310
	VK_Q VirtualKey = 301
	VK_R VirtualKey = 304
	VK_S VirtualKey = 402
	VK_T VirtualKey = 305
	VK_U VirtualKey = 307
	VK_V VirtualKey = 504
	VK_W VirtualKey = 302
	VK_X VirtualKey = 502
	VK_Y VirtualKey = 306
	VK_Z VirtualKey = 501

	// Symbol Keys
	VK_MINUS     VirtualKey = 211 // -
	VK_EQUALS    VirtualKey = 212 // =
	VK_LBRACKET  VirtualKey = 311 // [
	VK_RBRACKET  VirtualKey = 312 // ]
	VK_SEMICOLON VirtualKey = 410 // ;
	VK_QUOTE     VirtualKey = 411 // '
	VK_COMMA     VirtualKey = 508 // ,
	VK_PERIOD    VirtualKey = 509 // .
	VK_SLASH     VirtualKey = 510 // /
	VK_BACKSLASH VirtualKey = 213 // \
	VK_GRAVE     VirtualKey = 200 // `

	// Numpad
	VK_NUMPAD0      VirtualKey = 800
	VK_NUMPAD1      VirtualKey = 801
	VK_NUMPAD2      VirtualKey = 802
	VK_NUMPAD3      VirtualKey = 803
	VK_NUMPAD4      VirtualKey = 804
	VK_NUMPAD5      VirtualKey = 805
	VK_NUMPAD6      VirtualKey = 806
	VK_NUMPAD7      VirtualKey = 807
	VK_NUMPAD8      VirtualKey = 808
	VK_NUMPAD9      VirtualKey = 809
	VK_NUMPAD_DOT   VirtualKey = 816 // . in numpad
	VK_NUMPAD_ADD   VirtualKey = 814 // +
	VK_NUMPAD_SUB   VirtualKey = 813 // -
	VK_NUMPAD_MUL   VirtualKey = 812 // *
	VK_NUMPAD_DIV   VirtualKey = 811 // /
	VK_NUMPAD_ENTER VirtualKey = 815
)
