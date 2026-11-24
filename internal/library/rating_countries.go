// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"strings"
	"sync/atomic"
)

// Ratings per country. Metadata sources give a title's certificate for many countries; the
// server keeps the one for the country its owner chose (the "ratingCountry" setting), and the
// US one when that country has none. Boards in different countries reuse the same letters for
// different ages (an Australian M is 15, a New Zealand M is 16, an Indian A is 18 and a Spanish
// A is for everyone), so a certificate is read by its country's table first: the one written in
// front of it ("NZ:M"), or else the chosen country's.

// RatingCountryInfo is one country Settings offers.
type RatingCountryInfo struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// RatingCountries are the countries whose certificates can be kept, by name.
var RatingCountries = []RatingCountryInfo{
	{"AU", "Australia"}, {"BR", "Brazil"}, {"CA", "Canada"}, {"DK", "Denmark"}, {"FI", "Finland"}, {"FR", "France"},
	{"DE", "Germany"}, {"IN", "India"}, {"IE", "Ireland"}, {"IT", "Italy"}, {"JP", "Japan"}, {"MX", "Mexico"},
	{"NL", "Netherlands"}, {"NZ", "New Zealand"}, {"NO", "Norway"}, {"PT", "Portugal"}, {"KR", "South Korea"},
	{"ES", "Spain"}, {"SE", "Sweden"}, {"GB", "United Kingdom"}, {"US", "United States"},
}

// ValidRatingCountry reports whether a code is one of them.
func ValidRatingCountry(code string) bool {
	for _, c := range RatingCountries {
		if c.Code == code {
			return true
		}
	}
	return false
}

// countryAges are each country's certificates, written as AgeFor has them (upper case, no
// spaces, dashes or dots), where they differ from or add to ratingAges. A plain number is an
// age everywhere and needn't be listed.
var countryAges = map[string]map[string]int{
	"US": {"G": 0, "PG": 10, "PG13": 13, "R": 17, "NC17": 18, "TVY": 0, "TVY7": 7, "TVG": 0, "TVPG": 10, "TV14": 14, "TVMA": 17},
	"GB": {"U": 0, "UC": 0, "PG": 10, "12A": 12, "R18": 18},
	"DE": {"FSK0": 0, "FSK6": 6, "FSK12": 12, "FSK16": 16, "FSK18": 18},
	"FR": {"U": 0, "TP": 0, "TOUSPUBLICS": 0},
	"AU": {"E": 0, "G": 0, "PG": 10, "M": 15, "MA15+": 15, "R18+": 18, "X18+": 18},
	"NZ": {"G": 0, "PG": 10, "M": 16, "R13": 13, "RP13": 13, "R15": 15, "R16": 16, "RP16": 16, "R18": 18, "RP18": 18, "R": 18},
	"CA": {"G": 0, "PG": 10, "14A": 14, "18A": 18, "R": 18, "A": 18, "E": 0, "C": 0, "C8": 8},
	"IE": {"G": 0, "PG": 10, "12A": 12, "15A": 15},
	"ES": {"A": 0, "APTA": 0, "TP": 0, "X": 18},
	"IT": {"T": 0, "VM14": 14, "VM18": 18},
	"BR": {"L": 0, "AL": 0},
	"JP": {"G": 0, "PG12": 12, "R15+": 15, "R18+": 18},
	"KR": {"ALL": 0},
	"SE": {"BTL": 0},
	"NO": {"A": 0},
	"DK": {"A": 0},
	"FI": {"S": 0, "K7": 7, "K12": 12, "K16": 16, "K18": 18},
	"IN": {"U": 0, "UA": 12, "UA7+": 7, "UA13+": 13, "UA16+": 16, "A": 18, "S": 18},
	"MX": {"AA": 0, "A": 0, "B": 12, "B15": 15, "C": 18, "D": 18},
	"NL": {"AL": 0},
	"PT": {"M3": 3, "M6": 6, "M12": 12, "M14": 14, "M16": 16, "M18": 18},
}

// iso3 are the three-letter names some sources give the countries by.
var iso3 = map[string]string{"AU": "aus", "BR": "bra", "CA": "can", "DK": "dnk", "FI": "fin", "FR": "fra", "DE": "deu", "IN": "ind",
	"IE": "irl", "IT": "ita", "JP": "jpn", "MX": "mex", "NL": "nld", "NZ": "nzl", "NO": "nor", "PT": "prt", "KR": "kor", "ES": "esp",
	"SE": "swe", "GB": "gbr", "US": "usa"}

// RatingCountry says which country's certificates the server keeps. Set by the server from its
// setting; until then, and for anything it doesn't know, the US.
var RatingCountry func() string

var ratingCountryCache atomic.Value // string, so AgeFor doesn't read the database for every title

// SetRatingCountry is called when the setting is read or changed.
func SetRatingCountry(code string) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if !ValidRatingCountry(code) {
		code = "US"
	}
	ratingCountryCache.Store(code)
}

func ratingCountry() string {
	if c, ok := ratingCountryCache.Load().(string); ok && c != "" {
		return c
	}
	if RatingCountry != nil {
		if c := strings.ToUpper(RatingCountry()); ValidRatingCountry(c) {
			return c
		}
	}
	return "US"
}

// PickCertificate chooses a title's certificate from what a source gives by country: the
// chosen country's, or else the US one. get answers for one country, "" for none.
func PickCertificate(get func(iso string) string) string {
	c := ratingCountry()
	if v := strings.TrimSpace(get(c)); v != "" {
		return v
	}
	if c != "US" {
		return strings.TrimSpace(get("US"))
	}
	return ""
}

// countryPrefix splits "NZ:M" or "de/12" into the country and the rest, when the front is a
// country of RatingCountries.
func countryPrefix(r string) (country, rest string) {
	if len(r) > 3 && (r[2] == ':' || r[2] == '/') {
		c := strings.ToUpper(r[:2])
		if _, ok := countryAges[c]; ok || ValidRatingCountry(c) {
			return c, strings.TrimSpace(r[3:])
		}
	}
	return "", r
}

// RecomputeAges works out every title's minimum age again, after the country whose
// certificates are read changed what some of them mean.
func (s *Store) RecomputeAges(ctx context.Context) error {
	for _, table := range []string{"items", "shows"} {
		rows, err := s.DB.QueryContext(ctx, `SELECT id, content_rating, min_age FROM `+table+` WHERE IFNULL(content_rating, '') <> ''`)
		if err != nil {
			return err
		}
		type row struct {
			id  int64
			age *int
		}
		var todo []row
		for rows.Next() {
			var id int64
			var rating string
			var had *int
			if rows.Scan(&id, &rating, &had) != nil {
				continue
			}
			age := AgeFor(rating)
			if (age == nil) != (had == nil) || (age != nil && *age != *had) {
				todo = append(todo, row{id, age})
			}
		}
		rows.Close()
		for _, r := range todo {
			if _, err := s.DB.ExecContext(ctx, `UPDATE `+table+` SET min_age = ? WHERE id = ?`, r.age, r.id); err != nil {
				return err
			}
		}
	}
	return nil
}
