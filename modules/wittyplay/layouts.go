package wittyplay

// Controllers we know by vendor and product, so that common ones work with nothing to
// set up. A browser that puts a pad in standard order needs no help: all this does
// then is name its button style. The layouts below are for a browser that doesn't, as
// happens with cheap USB pads everywhere and with many pads on Linux. They're written
// by us from what these devices report, grouped by the order their firmware or driver
// lists buttons in; none of it is copied from another project's table.
//
// A client that isn't a browser reads its platform's own controller names and has no
// use for any of this.

// RawLayout puts a pad in standard order. Buttons is indexed by W3C standard number
// and says where each is really found; null means the pad has none. Axes are the left
// stick's X and Y and the right stick's X and Y, -1 for none. With no D-pad buttons
// listed (12 to 15), the D-pad is found by looking: a hat in the last axis, or else the
// last two axes.
type RawLayout struct {
	Buttons []*Binding `json:"buttons"`
	Axes    [4]int     `json:"axes"`
}

// KnownPad is a model. Layout applies to a browser that doesn't put it in standard
// order; LinuxLayout, when set, replaces it on Linux, where the kernel's driver chooses
// the order. Style says what its face buttons are called: xbox, playstation, nintendo
// or generic. It only changes words on the screen, as buttons are mapped by where they
// are.
type KnownPad struct {
	ID          string `json:"id"` // vendor:product, four lowercase hexadecimal digits each
	Name        string `json:"name"`
	Style       string `json:"style"`
	Layout      string `json:"layout,omitempty"`
	LinuxLayout string `json:"linuxLayout,omitempty"`
}

// ControllerData is everything a browser needs to read pads it can't assume anything
// about.
type ControllerData struct {
	Layouts map[string]RawLayout `json:"layouts"`
	Pads    []KnownPad           `json:"pads"`
}

func rb(n int) *Binding {
	b := button(n)
	return &b
}

func ra(n, dir int) *Binding {
	b := axis(n, dir)
	return &b
}

// rt is a trigger that rests at -1.
func rt(n int) *Binding {
	b := axis(n, 1)
	b.Rest = -1
	return &b
}

// faceOrder lists W3C buttons 0 to 3 by where each position is really found.
func faceOrder(bottom, right, left, top int) []*Binding {
	return []*Binding{rb(bottom), rb(right), rb(left), rb(top)}
}

// KnownControllers returns the table. It's small, and only sent when a client asks.
func KnownControllers() ControllerData {
	return ControllerData{
		Layouts: map[string]RawLayout{
			// The Linux driver for XInput pads: triggers are axes and the D-pad is the
			// last two.
			"linux-x": {Axes: [4]int{0, 1, 3, 4}, Buttons: append(faceOrder(0, 1, 2, 3),
				rb(4), rb(5), rt(2), rt(5), rb(6), rb(7), rb(9), rb(10), ra(7, -1), ra(7, 1), ra(6, -1), ra(6, 1), rb(8))},
			// The Linux drivers for two-stick console pads: the same shape, with Home
			// and the stick clicks a step along.
			"linux-sony": {Axes: [4]int{0, 1, 3, 4}, Buttons: append(faceOrder(0, 1, 3, 2),
				rb(4), rb(5), rt(2), rt(5), rb(8), rb(9), rb(11), rb(12), ra(7, -1), ra(7, 1), ra(6, -1), ra(6, 1), rb(10))},
			"linux-switch": {Axes: [4]int{0, 1, 3, 4}, Buttons: append(faceOrder(0, 1, 3, 2),
				rb(4), rb(5), rb(6), rb(7), rb(8), rb(9), rb(11), rb(12), ra(7, -1), ra(7, 1), ra(6, -1), ra(6, 1), rb(10))},
			// DirectInput's order on many pads: left, bottom, right, top, and the rest in
			// the standard order. The D-pad is usually a hat.
			"direct-lbrt": {Axes: [4]int{0, 1, 2, 3}, Buttons: append(faceOrder(1, 2, 0, 3),
				rb(4), rb(5), rb(6), rb(7), rb(8), rb(9), rb(10), rb(11), nil, nil, nil, nil, rb(12))},
			// The same, with the right stick's second axis where HID puts it.
			"direct-sony": {Axes: [4]int{0, 1, 2, 5}, Buttons: append(faceOrder(1, 2, 0, 3),
				rb(4), rb(5), rb(6), rb(7), rb(8), rb(9), rb(10), rb(11), nil, nil, nil, nil, rb(12))},
			// Cheap USB pads: top, right, bottom, left, counting clockwise.
			"usb-trbl": {Axes: [4]int{0, 1, 2, 3}, Buttons: append(faceOrder(2, 1, 3, 0),
				rb(4), rb(5), rb(6), rb(7), rb(8), rb(9), rb(10), rb(11))},
			// Four-button pads with no triggers or sticks.
			"usb-snes-trbl": {Axes: [4]int{-1, -1, -1, -1}, Buttons: append(faceOrder(2, 1, 3, 0),
				rb(4), rb(5), nil, nil, rb(8), rb(9))},
			"usb-snes-order": {Axes: [4]int{-1, -1, -1, -1}, Buttons: append(faceOrder(1, 0, 3, 2),
				rb(4), rb(5), nil, nil, rb(6), rb(7))},
			// USB pads with two face buttons and a D-pad on the first axes.
			"usb-nes": {Axes: [4]int{-1, -1, -1, -1}, Buttons: []*Binding{rb(1), rb(0), nil, nil, nil, nil, nil, nil, rb(2), rb(3)}},
		},
		Pads: []KnownPad{
			{ID: "045e:028e", Name: "Xbox 360 pad", Style: "xbox", LinuxLayout: "linux-x"},
			{ID: "045e:028f", Name: "Xbox 360 pad", Style: "xbox", LinuxLayout: "linux-x"},
			{ID: "045e:0291", Name: "Xbox 360 wireless receiver", Style: "xbox", LinuxLayout: "linux-x"},
			{ID: "045e:0719", Name: "Xbox 360 wireless receiver", Style: "xbox", LinuxLayout: "linux-x"},
			{ID: "045e:02d1", Name: "Xbox One pad", Style: "xbox", LinuxLayout: "linux-x"},
			{ID: "045e:02dd", Name: "Xbox One pad", Style: "xbox", LinuxLayout: "linux-x"},
			{ID: "045e:02e3", Name: "Xbox Elite pad", Style: "xbox", LinuxLayout: "linux-x"},
			{ID: "045e:02ea", Name: "Xbox One S pad", Style: "xbox", LinuxLayout: "linux-x"},
			{ID: "045e:02fd", Name: "Xbox One S pad", Style: "xbox", LinuxLayout: "linux-x"},
			{ID: "045e:0b12", Name: "Xbox Series pad", Style: "xbox", LinuxLayout: "linux-x"},
			{ID: "045e:0b13", Name: "Xbox Series pad", Style: "xbox", LinuxLayout: "linux-x"},
			{ID: "28de:11ff", Name: "Steam virtual pad", Style: "xbox", LinuxLayout: "linux-x"},
			{ID: "046d:c21d", Name: "Logitech F310", Style: "xbox", LinuxLayout: "linux-x"},
			{ID: "046d:c21e", Name: "Logitech F510", Style: "xbox", LinuxLayout: "linux-x"},
			{ID: "046d:c21f", Name: "Logitech F710", Style: "xbox", LinuxLayout: "linux-x"},
			{ID: "046d:c216", Name: "Logitech pad in D mode", Style: "generic", Layout: "direct-lbrt"},
			{ID: "046d:c218", Name: "Logitech pad in D mode", Style: "generic", Layout: "direct-lbrt"},
			{ID: "046d:c219", Name: "Logitech pad in D mode", Style: "generic", Layout: "direct-lbrt"},
			{ID: "054c:05c4", Name: "DualShock 4", Style: "playstation", Layout: "direct-sony", LinuxLayout: "linux-sony"},
			{ID: "054c:09cc", Name: "DualShock 4", Style: "playstation", Layout: "direct-sony", LinuxLayout: "linux-sony"},
			{ID: "054c:0ba0", Name: "DualShock 4 wireless adapter", Style: "playstation", Layout: "direct-sony", LinuxLayout: "linux-sony"},
			{ID: "054c:0ce6", Name: "DualSense", Style: "playstation", Layout: "direct-sony", LinuxLayout: "linux-sony"},
			{ID: "054c:0df2", Name: "DualSense Edge", Style: "playstation", Layout: "direct-sony", LinuxLayout: "linux-sony"},
			{ID: "054c:0268", Name: "DualShock 3", Style: "playstation", LinuxLayout: "linux-sony"},
			{ID: "057e:2009", Name: "Switch Pro pad", Style: "nintendo", LinuxLayout: "linux-switch"},
			{ID: "057e:2006", Name: "Left Joy-Con", Style: "nintendo"},
			{ID: "057e:2007", Name: "Right Joy-Con", Style: "nintendo"},
			{ID: "057e:200e", Name: "Joy-Con grip", Style: "nintendo", LinuxLayout: "linux-switch"},
			{ID: "18d1:9400", Name: "Stadia pad", Style: "generic"},
			{ID: "0079:0006", Name: "USB pad", Style: "generic", Layout: "usb-trbl"},
			{ID: "0079:0011", Name: "USB pad, four buttons", Style: "generic", Layout: "usb-snes-trbl"},
			{ID: "081f:e401", Name: "USB pad, four buttons", Style: "nintendo", Layout: "usb-snes-trbl"},
			{ID: "0810:e501", Name: "USB pad, four buttons", Style: "nintendo", Layout: "usb-snes-trbl"},
			{ID: "12bd:d015", Name: "USB pad, four buttons", Style: "nintendo", Layout: "usb-snes-trbl"},
			{ID: "0583:2060", Name: "USB pad, four buttons", Style: "nintendo", Layout: "usb-snes-order"},
			{ID: "0e8f:3013", Name: "USB adapter", Style: "generic", Layout: "usb-trbl"},
			{ID: "0810:0001", Name: "USB adapter", Style: "generic", Layout: "usb-trbl"},
			{ID: "0810:0003", Name: "USB adapter", Style: "generic", Layout: "usb-trbl"},
			{ID: "1781:0a9d", Name: "USB pad, two buttons", Style: "nintendo", Layout: "usb-nes"},
			{ID: "0e6f:0241", Name: "USB pad", Style: "xbox", LinuxLayout: "linux-x"},
			{ID: "0f0d:00c1", Name: "USB pad", Style: "nintendo", Layout: "direct-lbrt"},
			{ID: "2dc8:6101", Name: "USB pad", Style: "nintendo", Layout: "direct-lbrt"},
			{ID: "2dc8:9001", Name: "USB pad", Style: "nintendo", Layout: "direct-lbrt"},
			{ID: "1209:2882", Name: "USB pad", Style: "generic", Layout: "direct-lbrt"},
			{ID: "0955:7214", Name: "USB pad", Style: "generic", Layout: "direct-lbrt"},
		},
	}
}
