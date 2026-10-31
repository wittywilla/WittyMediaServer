package livetv

// What encrypted channels are and what someone can do about them. It comes from
// the server so that every client says the same thing. The device notes state
// only what the maker or the standards body publishes, and say when it was
// checked: keep docs/CHANNELS.md in step when changing them.

// HelpSection is one part of the help page.
type HelpSection struct {
	Heading    string   `json:"heading"`
	Paragraphs []string `json:"paragraphs,omitempty"`
	Bullets    []string `json:"bullets,omitempty"`
}

// HelpDevice is one thing checked in its maker's own documentation.
type HelpDevice struct {
	Name   string `json:"name"`
	Facts  string `json:"facts"`
	Source string `json:"source"`
	URL    string `json:"url"`
}

// Help is the page behind "Encrypted: can't be played here".
type Help struct {
	Title     string        `json:"title"`
	Sections  []HelpSection `json:"sections"`
	DevicesIn string        `json:"devicesIntro"`
	Devices   []HelpDevice  `json:"devices"`
	Checked   string        `json:"checked"` // when the device notes were checked
	Note      string        `json:"note"`
}

const helpChecked = "2026-10-30"

// EncryptedHelp is the help page's content.
func EncryptedHelp() Help {
	return Help{
		Title: "Encrypted channels",
		Sections: []HelpSection{
			{Heading: "What it means", Paragraphs: []string{
				"Some broadcasters encrypt their channels. The signal reaches your antenna or cable as usual, but only a device licensed for that encryption can turn it into a picture. Many ATSC 3.0 (NextGen TV) stations in the US do this, as do pay-TV channels and some internet streams.",
				"When a channel says “Encrypted: can't be played here”, nothing is wrong with your antenna, tuner or setup. The channel is simply protected.",
			}},
			{Heading: "Why WittyMediaServer can't play it", Paragraphs: []string{
				"WittyMediaServer isn't a device licensed to decrypt broadcasts, and it doesn't decrypt or bypass protection of any kind. It doesn't extract keys from a device, imitate a certified player, or capture protected (HDCP) HDMI. There is no setting or add-on that changes this.",
			}},
			{Heading: "What you can do", Bullets: []string{
				"Watch a clear version of the station. Many stations also air on ATSC 1.0 or another unencrypted source. If one of your tuners or playlists has the same station, the guide offers it beside the encrypted channel.",
				"Use a licensed device or app that is certified for encrypted channels, such as one of those below, to watch that channel.",
				"Bring in what a licensed device itself offers to home media servers. If a device documents a network tuner protocol, an HTTP or HLS stream, or a folder of exported files, add it here like any other source: under Settings → Channels & tuners, or as a library folder. If the stream can be played, it plays. If it is still encrypted when it reaches this server, it shows this same label.",
			}},
			{Heading: "What to look for in a device", Bullets: []string{
				"An ATSC 3.0 tuner, if the station is on ATSC 3.0.",
				"Certification for encrypted channels, stated by the maker. In the US this is the ATSC 3.0 Security Authority (A3SA) scheme.",
				"For use with this server: a documented network stream or home-media-server export, stated by the maker, that works for the channels you want. Check that it covers encrypted channels specifically. Many devices only pass those to their own apps.",
				"Any limits on recording. Broadcasters can restrict recording, and encrypted recordings may play only on the device that made them. This server honours any limit a device reports.",
			}},
		},
		DevicesIn: "What makers themselves publish. None of these is a recommendation, and support can change, so check with the maker before buying. As of this date, none of them documents a way to pass encrypted channels to other software such as this server.",
		Devices: []HelpDevice{
			{
				Name:   "SiliconDust HDHomeRun FLEX 4K",
				Facts:  "A network tuner with 2 ATSC 3.0 tuners and 4 ATSC 1.0 tuners. The maker says some DRM-encrypted ATSC 3.0 channels will not work, and that it uses the ATSC 1.0 version of such a channel. It documents an HTTP interface, and WittyMediaServer can use it as a network tuner for channels that are not encrypted.",
				Source: "SiliconDust product page and HTTP guide",
				URL:    "https://shop.silicondust.com/shop/product/hdfx-4k/",
			},
			{
				Name:   "ZapperBox",
				Facts:  "The maker says its tuner models work with encrypted channels and can record them, and that encrypted recordings play only on the device that made them. It documents APIs for finding a box on the network and for sending remote-control keys. Its plans for reaching other devices are through its own apps. Its free zApp tool converts recordings to standard HLS, and it says clear ATSC 3.0 recordings then play without sound in players without Dolby AC-4.",
				Source: "ZapperBox FAQ",
				URL:    "https://zapperbox.com/pages/faqs",
			},
			{
				Name:   "ADTH NextGen TV USB tuner",
				Facts:  "The maker calls it certified to the A3SA standard for playing encrypted channels online, on Android TV, Google TV and Fire TV devices, which do the decoding themselves. It documents no way for other software on the network to receive channels.",
				Source: "ADTH support",
				URL:    "https://support.adth.com/en/support/solutions/articles/43000754091-how-to-play-encrypted-channels-on-adth-nextgen-tv-usb-",
			},
			{
				Name:   "ADTH NextGen TV Box",
				Facts:  "The maker says it supports encrypted channels, without internet from software 2.0.0, and records to an attached USB drive from 2.0.4. It documents no way for other software on the network to receive channels.",
				Source: "ADTH support FAQ",
				URL:    "https://support.adth.com/en/support/solutions/articles/43000703336-faq",
			},
			{
				Name:   "NextGen TV sets",
				Facts:  "A3SA said in April 2026 that nearly 40 models from Hisense, Panasonic, Samsung, Sony and TCL can decrypt protected content. Check the particular model with its maker.",
				Source: "A3SA news",
				URL:    "https://a3sa.com/news-updates/",
			},
		},
		Checked: helpChecked,
		Note:    "No maker here endorses this software, and none has been paid or asked to appear.",
	}
}
