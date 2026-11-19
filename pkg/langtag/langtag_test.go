// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package langtag

import "testing"

func TestCanonical(t *testing.T) {
	for in, want := range map[string]string{
		"de": "de", "GER": "de", "deu": "de", "pt_br": "pt-BR", "chi-hant": "zh-Hant", "zh-Hant-TW": "zh-Hant-TW", "yue": "yue",
		"sr_latn": "sr-Latn", "es-419": "es-419", "iw": "he", "fil": "fil", "kat": "ka", "geo": "ka", "qaa": "qaa", "tlh": "tlh", "en-gb": "en-GB",
	} {
		if got, ok := Canonical(in); !ok || got != want {
			t.Errorf("%q: %q %v, want %q", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "english", "e", "de--AT", "de AT", "../x", "a1", "toolongtagvalue"} {
		if _, ok := Canonical(bad); ok {
			t.Errorf("%q taken as a tag", bad)
		}
	}
}

func TestEveryISO6391LanguageIsKnown(t *testing.T) {
	if n := len(All()); n < 184+15 {
		t.Fatalf("only %d languages", n)
	}
	seen := map[string]bool{}
	for _, l := range table {
		if seen[l.code] {
			t.Errorf("%s twice", l.code)
		}
		seen[l.code] = true
		for _, o := range l.other {
			if Base(o) != l.code {
				t.Errorf("%s doesn't come back to %s", o, l.code)
			}
		}
		if l.name == "" {
			t.Errorf("%s has no name", l.code)
		}
	}
}

func TestSameAndBest(t *testing.T) {
	if !Same("ger", "de-AT") || Same("und", "und") || Same("pt", "es") {
		t.Error("Same")
	}
	have := []string{"pt-PT", "pt", "en", "pt-BR"}
	if Best("pt-BR", have) != "pt-BR" || Best("pt-AO", have) != "pt" || Best("por", have) != "pt" || Best("ja", have) != "" {
		t.Error(Best("pt-BR", have), Best("pt-AO", have), Best("por", have))
	}
	if Name("chi") != "Chinese" || Name("yue") != "Cantonese" || Name("xx") != "" {
		t.Error(Name("chi"), Name("yue"))
	}
}
