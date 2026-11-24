// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package langtag reads language codes in every form media carries them: ISO 639-1 ("de"),
// ISO 639-2 in both its bibliographic and terminology forms ("ger", "deu"), ISO 639-3 and
// BCP 47 tags with a script or region ("pt-BR", "zh-Hant-TW", "sr_Latn"). It knows every
// ISO 639-1 language, as Qt's QLocale (WittyMediaPlayer) and the browsers' Intl do, and the
// three-letter codes media most often has without a two-letter one. A code it doesn't know
// but that is well formed is kept as it is: a tag is never refused for being rare.
package langtag

import (
	"github.com/wittywilla/wittymediaserver/internal/i18n"
	"regexp"
	"sort"
	"strings"
)

// lang is one language: its two-letter code (or three-letter, for one with none), its other
// codes, and its English name.
type lang struct {
	code  string
	other []string
	name  string
}

// table: every ISO 639-1 language, with its ISO 639-2/T code and, where different, /B.
var table = []lang{
	{"aa", []string{"aar"}, "Afar"}, {"ab", []string{"abk"}, "Abkhazian"}, {"ae", []string{"ave"}, "Avestan"}, {"af", []string{"afr"}, "Afrikaans"},
	{"ak", []string{"aka"}, "Akan"}, {"am", []string{"amh"}, "Amharic"}, {"an", []string{"arg"}, "Aragonese"}, {"ar", []string{"ara"}, "Arabic"},
	{"as", []string{"asm"}, "Assamese"}, {"av", []string{"ava"}, "Avaric"}, {"ay", []string{"aym"}, "Aymara"}, {"az", []string{"aze"}, "Azerbaijani"},
	{"ba", []string{"bak"}, "Bashkir"}, {"be", []string{"bel"}, "Belarusian"}, {"bg", []string{"bul"}, "Bulgarian"}, {"bh", []string{"bih"}, "Bihari"},
	{"bi", []string{"bis"}, "Bislama"}, {"bm", []string{"bam"}, "Bambara"}, {"bn", []string{"ben"}, "Bangla"}, {"bo", []string{"bod", "tib"}, "Tibetan"},
	{"br", []string{"bre"}, "Breton"}, {"bs", []string{"bos"}, "Bosnian"}, {"ca", []string{"cat"}, "Catalan"}, {"ce", []string{"che"}, "Chechen"},
	{"ch", []string{"cha"}, "Chamorro"}, {"co", []string{"cos"}, "Corsican"}, {"cr", []string{"cre"}, "Cree"}, {"cs", []string{"ces", "cze"}, "Czech"},
	{"cu", []string{"chu"}, "Church Slavic"}, {"cv", []string{"chv"}, "Chuvash"}, {"cy", []string{"cym", "wel"}, "Welsh"}, {"da", []string{"dan"}, "Danish"},
	{"de", []string{"deu", "ger"}, "German"}, {"dv", []string{"div"}, "Divehi"}, {"dz", []string{"dzo"}, "Dzongkha"}, {"ee", []string{"ewe"}, "Ewe"},
	{"el", []string{"ell", "gre"}, "Greek"}, {"en", []string{"eng"}, "English"}, {"eo", []string{"epo"}, "Esperanto"}, {"es", []string{"spa"}, "Spanish"},
	{"et", []string{"est"}, "Estonian"}, {"eu", []string{"eus", "baq"}, "Basque"}, {"fa", []string{"fas", "per"}, "Persian"}, {"ff", []string{"ful"}, "Fula"},
	{"fi", []string{"fin"}, "Finnish"}, {"fj", []string{"fij"}, "Fijian"}, {"fo", []string{"fao"}, "Faroese"}, {"fr", []string{"fra", "fre"}, "French"},
	{"fy", []string{"fry"}, "Western Frisian"}, {"ga", []string{"gle"}, "Irish"}, {"gd", []string{"gla"}, "Scottish Gaelic"}, {"gl", []string{"glg"}, "Galician"},
	{"gn", []string{"grn"}, "Guarani"}, {"gu", []string{"guj"}, "Gujarati"}, {"gv", []string{"glv"}, "Manx"}, {"ha", []string{"hau"}, "Hausa"},
	{"he", []string{"heb", "iw"}, "Hebrew"}, {"hi", []string{"hin"}, "Hindi"}, {"ho", []string{"hmo"}, "Hiri Motu"}, {"hr", []string{"hrv"}, "Croatian"},
	{"ht", []string{"hat"}, "Haitian Creole"}, {"hu", []string{"hun"}, "Hungarian"}, {"hy", []string{"hye", "arm"}, "Armenian"}, {"hz", []string{"her"}, "Herero"},
	{"ia", []string{"ina"}, "Interlingua"}, {"id", []string{"ind", "in"}, "Indonesian"}, {"ie", []string{"ile"}, "Interlingue"}, {"ig", []string{"ibo"}, "Igbo"},
	{"ii", []string{"iii"}, "Sichuan Yi"}, {"ik", []string{"ipk"}, "Inupiaq"}, {"io", []string{"ido"}, "Ido"}, {"is", []string{"isl", "ice"}, "Icelandic"},
	{"it", []string{"ita"}, "Italian"}, {"iu", []string{"iku"}, "Inuktitut"}, {"ja", []string{"jpn"}, "Japanese"}, {"jv", []string{"jav"}, "Javanese"},
	{"ka", []string{"kat", "geo"}, "Georgian"}, {"kg", []string{"kon"}, "Kongo"}, {"ki", []string{"kik"}, "Kikuyu"}, {"kj", []string{"kua"}, "Kuanyama"},
	{"kk", []string{"kaz"}, "Kazakh"}, {"kl", []string{"kal"}, "Kalaallisut"}, {"km", []string{"khm"}, "Khmer"}, {"kn", []string{"kan"}, "Kannada"},
	{"ko", []string{"kor"}, "Korean"}, {"kr", []string{"kau"}, "Kanuri"}, {"ks", []string{"kas"}, "Kashmiri"}, {"ku", []string{"kur"}, "Kurdish"},
	{"kv", []string{"kom"}, "Komi"}, {"kw", []string{"cor"}, "Cornish"}, {"ky", []string{"kir"}, "Kyrgyz"}, {"la", []string{"lat"}, "Latin"},
	{"lb", []string{"ltz"}, "Luxembourgish"}, {"lg", []string{"lug"}, "Ganda"}, {"li", []string{"lim"}, "Limburgish"}, {"ln", []string{"lin"}, "Lingala"},
	{"lo", []string{"lao"}, "Lao"}, {"lt", []string{"lit"}, "Lithuanian"}, {"lu", []string{"lub"}, "Luba-Katanga"}, {"lv", []string{"lav"}, "Latvian"},
	{"mg", []string{"mlg"}, "Malagasy"}, {"mh", []string{"mah"}, "Marshallese"}, {"mi", []string{"mri", "mao"}, "Māori"}, {"mk", []string{"mkd", "mac"}, "Macedonian"},
	{"ml", []string{"mal"}, "Malayalam"}, {"mn", []string{"mon"}, "Mongolian"}, {"mr", []string{"mar"}, "Marathi"}, {"ms", []string{"msa", "may"}, "Malay"},
	{"mt", []string{"mlt"}, "Maltese"}, {"my", []string{"mya", "bur"}, "Burmese"}, {"na", []string{"nau"}, "Nauru"}, {"nb", []string{"nob"}, "Norwegian Bokmål"},
	{"nd", []string{"nde"}, "North Ndebele"}, {"ne", []string{"nep"}, "Nepali"}, {"ng", []string{"ndo"}, "Ndonga"}, {"nl", []string{"nld", "dut"}, "Dutch"},
	{"nn", []string{"nno"}, "Norwegian Nynorsk"}, {"no", []string{"nor"}, "Norwegian"}, {"nr", []string{"nbl"}, "South Ndebele"}, {"nv", []string{"nav"}, "Navajo"},
	{"ny", []string{"nya"}, "Nyanja"}, {"oc", []string{"oci"}, "Occitan"}, {"oj", []string{"oji"}, "Ojibwa"}, {"om", []string{"orm"}, "Oromo"},
	{"or", []string{"ori"}, "Odia"}, {"os", []string{"oss"}, "Ossetic"}, {"pa", []string{"pan"}, "Punjabi"}, {"pi", []string{"pli"}, "Pali"},
	{"pl", []string{"pol"}, "Polish"}, {"ps", []string{"pus"}, "Pashto"}, {"pt", []string{"por"}, "Portuguese"}, {"qu", []string{"que"}, "Quechua"},
	{"rm", []string{"roh"}, "Romansh"}, {"rn", []string{"run"}, "Rundi"}, {"ro", []string{"ron", "rum", "mo", "mol"}, "Romanian"}, {"ru", []string{"rus"}, "Russian"},
	{"rw", []string{"kin"}, "Kinyarwanda"}, {"sa", []string{"san"}, "Sanskrit"}, {"sc", []string{"srd"}, "Sardinian"}, {"sd", []string{"snd"}, "Sindhi"},
	{"se", []string{"sme"}, "Northern Sami"}, {"sg", []string{"sag"}, "Sango"}, {"si", []string{"sin"}, "Sinhala"}, {"sk", []string{"slk", "slo"}, "Slovak"},
	{"sl", []string{"slv"}, "Slovenian"}, {"sm", []string{"smo"}, "Samoan"}, {"sn", []string{"sna"}, "Shona"}, {"so", []string{"som"}, "Somali"},
	{"sq", []string{"sqi", "alb"}, "Albanian"}, {"sr", []string{"srp"}, "Serbian"}, {"ss", []string{"ssw"}, "Swati"}, {"st", []string{"sot"}, "Southern Sotho"},
	{"su", []string{"sun"}, "Sundanese"}, {"sv", []string{"swe"}, "Swedish"}, {"sw", []string{"swa"}, "Swahili"}, {"ta", []string{"tam"}, "Tamil"},
	{"te", []string{"tel"}, "Telugu"}, {"tg", []string{"tgk"}, "Tajik"}, {"th", []string{"tha"}, "Thai"}, {"ti", []string{"tir"}, "Tigrinya"},
	{"tk", []string{"tuk"}, "Turkmen"}, {"tl", []string{"tgl"}, "Tagalog"}, {"tn", []string{"tsn"}, "Tswana"}, {"to", []string{"ton"}, "Tongan"},
	{"tr", []string{"tur"}, "Turkish"}, {"ts", []string{"tso"}, "Tsonga"}, {"tt", []string{"tat"}, "Tatar"}, {"tw", []string{"twi"}, "Twi"},
	{"ty", []string{"tah"}, "Tahitian"}, {"ug", []string{"uig"}, "Uyghur"}, {"uk", []string{"ukr"}, "Ukrainian"}, {"ur", []string{"urd"}, "Urdu"},
	{"uz", []string{"uzb"}, "Uzbek"}, {"ve", []string{"ven"}, "Venda"}, {"vi", []string{"vie"}, "Vietnamese"}, {"vo", []string{"vol"}, "Volapük"},
	{"wa", []string{"wln"}, "Walloon"}, {"wo", []string{"wol"}, "Wolof"}, {"xh", []string{"xho"}, "Xhosa"}, {"yi", []string{"yid", "ji"}, "Yiddish"},
	{"yo", []string{"yor"}, "Yoruba"}, {"za", []string{"zha"}, "Zhuang"}, {"zh", []string{"zho", "chi"}, "Chinese"}, {"zu", []string{"zul"}, "Zulu"},
	// Three-letter codes with no two-letter one that media often carries.
	{"ast", nil, "Asturian"}, {"ceb", nil, "Cebuano"}, {"cmn", nil, "Mandarin Chinese"}, {"fil", nil, "Filipino"}, {"gsw", nil, "Swiss German"},
	{"haw", nil, "Hawaiian"}, {"hmn", nil, "Hmong"}, {"kab", nil, "Kabyle"}, {"mni", nil, "Manipuri"}, {"nan", nil, i18n.T("Min Nan Chinese")},
	{"nds", nil, "Low German"}, {"sco", nil, "Scots"}, {"tlh", nil, "Klingon"}, {"yue", nil, "Cantonese"}, {"wuu", nil, "Wu Chinese"},
	{"mul", nil, "Multiple languages"}, {"zxx", nil, "No language"}, {"und", nil, "Undetermined"}, {"mis", nil, "Uncoded language"},
}

var (
	byCode  = map[string]*lang{}
	short   = map[string]string{} // any known code to its short form
	tagRe   = regexp.MustCompile(`^[A-Za-z]{2,8}([-_][A-Za-z0-9]{1,8})*$`)
	scripts = regexp.MustCompile(`^[A-Za-z]{4}$`)
	regions = regexp.MustCompile(`^([A-Za-z]{2}|[0-9]{3})$`)
)

func init() {
	for i := range table {
		l := &table[i]
		byCode[l.code] = l
		short[l.code] = l.code
		for _, o := range l.other {
			short[o] = l.code
		}
	}
}

// Canonical is a tag in its usual form: the shortest code for the language, a script in title
// case and a region in capitals ("GER" is "de", "pt_br" is "pt-BR", "chi-hant" is "zh-Hant").
// ok is false for something that isn't a language tag at all; a well formed one this package
// doesn't know is kept, lower-cased.
func Canonical(tag string) (string, bool) {
	tag = strings.TrimSpace(tag)
	if tag == "" || len(tag) > 35 || !tagRe.MatchString(tag) {
		return "", false
	}
	parts := strings.FieldsFunc(tag, func(r rune) bool { return r == '-' || r == '_' })
	base := strings.ToLower(parts[0])
	if len(base) > 3 && len(parts) == 1 {
		return "", false // a word, not a code
	}
	if s, ok := short[base]; ok {
		base = s
	}
	out := []string{base}
	for _, p := range parts[1:] {
		switch {
		case scripts.MatchString(p):
			out = append(out, strings.ToUpper(p[:1])+strings.ToLower(p[1:]))
		case regions.MatchString(p):
			out = append(out, strings.ToUpper(p))
		default:
			out = append(out, strings.ToLower(p))
		}
	}
	return strings.Join(out, "-"), true
}

// Valid says whether tag is a language tag.
func Valid(tag string) bool { _, ok := Canonical(tag); return ok }

// Base is the language alone, in its shortest code: "pt-BR" is "pt", "ger" is "de". "" for
// something that isn't a tag.
func Base(tag string) string {
	c, ok := Canonical(tag)
	if !ok {
		return ""
	}
	if i := strings.IndexByte(c, '-'); i > 0 {
		return c[:i]
	}
	return c
}

// Known says whether the language is one this package can name.
func Known(tag string) bool { _, ok := byCode[Base(tag)]; return ok }

// Name is the language's English name, "" when it isn't known.
func Name(tag string) string {
	if l, ok := byCode[Base(tag)]; ok {
		return l.name
	}
	return ""
}

// Same says whether two tags are the same language (whatever the region or script):
// "ger", "de" and "de-AT" are.
func Same(a, b string) bool {
	x, y := Base(a), Base(b)
	return x != "" && x != "und" && x == y
}

// Best picks the tag in have that suits want: the same tag, else the same language and
// script, else the same language. "" when none does.
func Best(want string, have []string) string {
	w, ok := Canonical(want)
	if !ok {
		return ""
	}
	score := func(h string) int {
		c, ok := Canonical(h)
		switch {
		case !ok:
			return 0
		case strings.EqualFold(c, w):
			return 3
		case Base(c) != Base(w):
			return 0
		case c == Base(c):
			return 2 // the plain language: a fair answer for any region
		}
		return 1
	}
	best, bestScore := "", 0
	for _, h := range have {
		if s := score(h); s > bestScore {
			best, bestScore = h, s
		}
	}
	return best
}

// All lists every language this package knows by its short code, in code order.
func All() []string {
	out := make([]string, 0, len(table))
	for _, l := range table {
		out = append(out, l.code)
	}
	sort.Strings(out)
	return out
}
