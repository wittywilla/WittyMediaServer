package wittymoonlight

// Credit says what a piece is, under what licence, and where its source is, for anywhere that
// shows licences. Being clear about this is part of using other people's work properly.
type Credit struct {
	Name    string `json:"name"`
	Role    string `json:"role"`
	License string `json:"license"`
	Source  string `json:"source"`
}

// Credits lists every piece a game stream goes through.
func Credits() []Credit {
	return []Credit{
		{"WittyMoonlight", "The relay in this server, and the client in your browser", "Apache-2.0", "https://github.com/wittywilla/WittyMoonlight"},
		{"WittyMoonlight Host", "A separate program that speaks to the PC, built on Moonlight's own code", "GPL-3.0", "https://github.com/wittywilla/WittyMoonlightHost"},
		{"moonlight-common-c", "Moonlight's streaming core, inside WittyMoonlight Host", "GPL-3.0", "https://github.com/moonlight-stream/moonlight-common-c"},
		{"ENet", "Reliable messages over UDP, inside moonlight-common-c", "MIT", "https://github.com/cgutman/enet"},
		{"nanors", "Repairs lost video packets, inside moonlight-common-c", "MIT", "https://github.com/sleepybishop/nanors"},
		{"Mbed TLS", "Encryption for the stream, inside WittyMoonlight Host", "Apache-2.0", "https://github.com/Mbed-TLS/mbedtls"},
		{"Pion WebRTC", "Carries the stream from this server to your browser", "MIT", "https://github.com/pion/webrtc"},
		{"Sunshine", "Runs on the PC you connect to, installed by whoever owns it", "GPL-3.0", "https://github.com/LizardByte/Sunshine"},
	}
}
