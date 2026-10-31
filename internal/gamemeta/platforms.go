package gamemeta

import "strconv"

// Each source's own number for each console. A console a table leaves out is simply
// searched by name alone. The console ids are wittyplay's; ones it doesn't have yet are
// here so that adding a console doesn't mean coming back to this file. "pc" is a gaming
// PC's games, which have no ROM to identify and only a name to go on.
//
// One of ours can be two of theirs: wittyplay keeps Game Boy and Game Boy Color together.

type platformIDs struct {
	igdb, rawg, tgdb, ss []int
	wikidata             []string // items a game's "platform" statement points at
}

var platforms = map[string]platformIDs{
	"nes":    {[]int{18, 99, 51}, []int{49}, []int{7, 4936}, []int{3, 106}, []string{"Q172742", "Q491640", "Q135321"}},
	"snes":   {[]int{19, 58}, []int{79}, []int{6}, []int{4}, []string{"Q183259"}},
	"n64":    {[]int{4}, []int{83}, []int{3}, []int{14}, []string{"Q184839"}},
	"gc":     {[]int{21}, []int{105}, []int{2}, []int{13}, []string{"Q182172"}},
	"wii":    {[]int{5}, []int{11}, []int{9}, []int{16}, []string{"Q8079"}},
	"wiiu":   {[]int{41}, []int{10}, []int{38}, []int{18}, []string{"Q56942"}},
	"gb":     {[]int{33, 22}, []int{26, 43}, []int{4, 41}, []int{9, 10}, []string{"Q186437", "Q203992"}},
	"gba":    {[]int{24}, []int{24}, []int{5}, []int{12}, []string{"Q188642"}},
	"nds":    {[]int{20}, []int{9}, []int{8}, []int{15}, []string{"Q170323"}},
	"3ds":    {[]int{37}, []int{8}, []int{4912}, []int{17}, []string{"Q203597"}},
	"md":     {[]int{29}, []int{167}, []int{18}, []int{1}, []string{"Q10676"}},
	"sms":    {[]int{64}, []int{74}, []int{35}, []int{2}, []string{"Q209868"}},
	"gg":     {[]int{35}, []int{77}, []int{20}, []int{21}, []string{"Q751719"}},
	"segacd": {[]int{78}, []int{119}, []int{21}, []int{20}, []string{"Q1047516"}},
	"32x":    {[]int{30}, nil, []int{33}, []int{19}, []string{"Q1063978"}},
	"saturn": {[]int{32}, []int{107}, []int{17}, []int{22}, []string{"Q200912"}},
	"dc":     {[]int{23}, []int{106}, []int{16}, []int{23}, []string{"Q184198"}},
	"ps1":    {[]int{7}, []int{27}, []int{10}, []int{57}, []string{"Q10677"}},
	"ps2":    {[]int{8}, []int{15}, []int{11}, []int{58}, []string{"Q10680"}},
	"psp":    {[]int{38}, []int{17}, []int{13}, []int{61}, []string{"Q170325"}},
	"pce":    {[]int{86, 150}, nil, []int{34, 4955}, []int{31, 114}, []string{"Q1057377", "Q10854461"}},
	"a2600":  {[]int{59}, []int{23}, []int{22}, []int{26}, []string{"Q206261"}},
	"a5200":  {[]int{66}, []int{31}, []int{26}, []int{40}, []string{"Q743222"}},
	"a7800":  {[]int{60}, []int{28}, []int{27}, []int{41}, []string{"Q753600"}},
	"lynx":   {[]int{61}, []int{46}, []int{4924}, []int{28}, []string{"Q753657"}},
	"jaguar": {[]int{62}, []int{112}, []int{28}, []int{27}, []string{"Q650601"}},
	"ws":     {[]int{57, 123}, nil, []int{4925, 4926}, []int{45, 46}, []string{"Q1065792", "Q1048035"}},
	"ngp":    {[]int{119, 120}, nil, []int{4922, 4923}, []int{25, 82}, []string{"Q939881", "Q1977455"}},
	"vb":     {[]int{87}, nil, []int{4918}, []int{11}, []string{"Q164651"}},
	"neogeo": {[]int{80}, []int{12}, []int{24}, []int{142}, []string{"Q1054350", "Q64428080", "Q3338058"}},
	"arcade": {[]int{52}, nil, []int{23}, []int{75}, []string{"Q192851"}},
	"pc":     {[]int{6, 13}, []int{4}, []int{1}, []int{138, 135}, nil},
	"rig":    {[]int{6, 13}, []int{4}, []int{1}, []int{138, 135}, nil}, // a game on a gaming PC
}

// PlatformKnown says whether we can name this console to sources.
func PlatformKnown(console string) bool {
	_, ok := platforms[console]
	return ok
}

func joinInts(v []int, sep string) string {
	out := ""
	for i, n := range v {
		if i > 0 {
			out += sep
		}
		out += strconv.Itoa(n)
	}
	return out
}
