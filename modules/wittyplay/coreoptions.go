package wittyplay

// coreOptions are the settings a core starts with here when its own default doesn't
// suit streaming. Nestopia's default filter imitates a TV signal: it triples the
// width, makes every frame different and blurs the pixels the encoder works to keep
// sharp. Supafaust pins its two threads to the first two CPUs, where every game on
// the server and everything else would then be fighting for them.
var coreOptions = map[string]map[string]string{
	"nestopia":           {"nestopia_blargg_ntsc_filter": "disabled"},
	"mednafen_supafaust": {"supafaust_thread_affinity_emu": "0x0", "supafaust_thread_affinity_ppu": "0x0"},
}

// CoreOptions returns a copy of a core's starting settings, nil when it has none.
func CoreOptions(id string) map[string]string {
	src := coreOptions[id]
	if src == nil {
		return nil
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}
