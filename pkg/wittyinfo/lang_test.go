// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittyinfo

import (
	"strings"
	"testing"
)

const translated = `{
  "format": "wittyinfo", "version": 1, "language": "eng",
  "segments": [{"id": "a", "type": "custom", "start": 10, "end": 20, "label": "Dream sequence"}],
  "scenes": [{"id": "s1", "title": "The heist", "start": 60}],
  "filters": [{"id": "f1", "category": "language", "start": 5, "end": 6, "label": "A swear word"}],
  "metadata": {"title": "The Film", "overview": "A film.", "languages": ["jpn", "en_us"]},
  "playback": {"audioLanguage": "ger", "subtitleLanguage": "pt_br"},
  "translations": {
    "ger": {"segments": {"a": {"label": "Traumsequenz"}}, "scenes": {"s1": {"title": "Der Raub"}}, "metadata": {"title": "Der Film"}},
    "pt-BR": {"segments": {"a": {"label": "Sequência do sonho"}}, "metadata": {"overview": "Um filme."}},
    "ja": {"scenes": {"s1": {"title": "強盗"}}, "notes": "メモ"}
  }
}`

func TestLanguagesAreReadAndWrittenInTheirUsualForm(t *testing.T) {
	x, err := Parse([]byte(translated))
	if err != nil {
		t.Fatal(problemsOf(err))
	}
	b, err := x.Encode()
	if err != nil {
		t.Fatal(problemsOf(err))
	}
	y, _ := Parse(b)
	if y.Language != "en" || y.Playback.AudioLanguage != "de" || y.Playback.SubtitleLanguage != "pt-BR" || strings.Join(y.Metadata.Languages, ",") != "ja,en-US" {
		t.Errorf("%q %+v %v", y.Language, y.Playback, y.Metadata.Languages)
	}
	if strings.Join(y.TranslationLanguages(), ",") != "en,de,ja,pt-BR" {
		t.Errorf("%v", y.TranslationLanguages())
	}
}

func TestLocalize(t *testing.T) {
	x, err := Parse([]byte(translated))
	if err != nil {
		t.Fatal(problemsOf(err))
	}
	de := x.Localize("de-AT")
	if de.Segments[0].Label != "Traumsequenz" || de.Scenes[0].Title != "Der Raub" || de.Metadata.Title != "Der Film" || de.Metadata.Overview != "A film." || de.Language != "ger" {
		t.Errorf("de: %+v %+v %+v", de.Segments[0], de.Scenes[0], de.Metadata)
	}
	if x.Segments[0].Label != "Dream sequence" {
		t.Error("Localize changed the file itself")
	}
	pt := x.Localize("por")
	if pt.Segments[0].Label != "Sequência do sonho" || pt.Scenes[0].Title != "The heist" || pt.Metadata.Overview != "Um filme." {
		t.Errorf("pt: %+v", pt.Segments[0])
	}
	if ja := x.Localize("jpn"); ja.Scenes[0].Title != "強盗" || ja.Notes != "メモ" || ja.Segments[0].Label != "Dream sequence" {
		t.Errorf("ja: %+v", ja.Scenes[0])
	}
	for _, same := range []string{"en-GB", "eng", "fr", "", "nonsense!"} {
		if x.Localize(same) != x {
			t.Errorf("%q should leave the file's own text", same)
		}
	}
}

func TestBadTranslationsAreRefused(t *testing.T) {
	man := func(extra string) []byte {
		return []byte(`{"format":"wittyinfo","version":1,"segments":[{"id":"a","type":"intro","start":1,"end":2}]` + extra + `}`)
	}
	for name, b := range map[string][]byte{
		"bad language":   man(`,"language":"English"`),
		"bad key":        man(`,"translations":{"!!":{}}`),
		"same twice":     man(`,"translations":{"de":{"notes":"a"},"ger":{"notes":"b"}}`),
		"unknown part":   man(`,"translations":{"de":{"segments":{"zzz":{"label":"x"}}}}`),
		"bad track lang": man(`,"playback":{"audioLanguage":"german language"}`),
		"long label":     man(`,"translations":{"de":{"segments":{"a":{"label":"` + strings.Repeat("x", 201) + `"}}}}`),
	} {
		if _, err := Parse(b); err == nil {
			t.Errorf("%s: read without complaint", name)
		}
	}
}
