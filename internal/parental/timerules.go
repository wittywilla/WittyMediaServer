// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package parental holds the parts of parental controls that are plain
// arithmetic: when a profile may watch, how long it has left, and what each
// country's ratings mean in years. It knows nothing of the database or of
// HTTP, so every rule here can be tested with a clock it is handed.
package parental

import (
	"errors"
	"fmt"
	"time"
)

// TimeRules say when a profile may play things, and for how long each day.
// The zero value is no rule at all.
type TimeRules struct {
	// Minutes a day: on school days (Monday to Friday) and at the weekend.
	// 0 = no allowance, so only the hours below apply.
	SchoolDayMinutes int `json:"schoolDayMinutes"`
	WeekendMinutes   int `json:"weekendMinutes"`
	// From is the earliest time of day, as "07:00". SchoolNightUntil is bedtime
	// on the evening before a school day (Sunday to Thursday) and WeekendUntil
	// on Friday and Saturday. "" = no such limit.
	From             string `json:"from"`
	SchoolNightUntil string `json:"schoolNightUntil"`
	WeekendUntil     string `json:"weekendUntil"`
}

// MaxDayMinutes is a whole day: an allowance can't be more.
const MaxDayMinutes = 24 * 60

// defaultFrom is when the day starts for a profile that has a bedtime and no
// earliest time. Without it bedtime would end at midnight.
const defaultFrom = 6 * 60

// Empty reports whether the rules limit nothing.
func (t *TimeRules) Empty() bool {
	return t == nil || (t.SchoolDayMinutes <= 0 && t.WeekendMinutes <= 0 && t.From == "" && t.SchoolNightUntil == "" && t.WeekendUntil == "")
}

// Valid says what is wrong with rules someone typed, or nil.
func (t *TimeRules) Valid() error {
	if t == nil {
		return nil
	}
	for _, m := range []int{t.SchoolDayMinutes, t.WeekendMinutes} {
		if m < 0 || m > MaxDayMinutes {
			return errors.New("minutes a day must be between 0 and 1440")
		}
	}
	from := -1
	if t.From != "" {
		m, ok := clock(t.From)
		if !ok {
			return fmt.Errorf("%q isn't a time of day, such as 07:00", t.From)
		}
		from = m
	}
	for _, u := range []string{t.SchoolNightUntil, t.WeekendUntil} {
		if u == "" {
			continue
		}
		m, ok := clock(u)
		if !ok {
			return fmt.Errorf("%q isn't a time of day, such as 20:30", u)
		}
		start := from
		if start < 0 {
			start = defaultFrom
		}
		if m <= start {
			return errors.New("bedtime has to be later in the day than the earliest time")
		}
	}
	return nil
}

// clock reads "HH:MM" as minutes into the day.
func clock(s string) (int, bool) {
	if len(s) != 5 || s[2] != ':' {
		return 0, false
	}
	for _, i := range []int{0, 1, 3, 4} {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
	}
	h, m := int(s[0]-'0')*10+int(s[1]-'0'), int(s[3]-'0')*10+int(s[4]-'0')
	if h > 23 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

func schoolDay(d time.Weekday) bool { return d >= time.Monday && d <= time.Friday }

// AllowanceSec is how long the profile may play on the day of now, in
// seconds. -1 = no allowance that day.
func (t *TimeRules) AllowanceSec(now time.Time) int {
	if t == nil {
		return -1
	}
	m := t.WeekendMinutes
	if schoolDay(now.Weekday()) {
		m = t.SchoolDayMinutes
	}
	if m <= 0 {
		return -1
	}
	return m * 60
}

// window is the hours of the day of now in which playing is allowed. ok is
// false when no hours are set.
func (t *TimeRules) window(now time.Time) (from, until time.Time, ok bool) {
	if t == nil || (t.From == "" && t.SchoolNightUntil == "" && t.WeekendUntil == "") {
		return from, until, false
	}
	y, mo, d := now.Date()
	midnight := time.Date(y, mo, d, 0, 0, 0, 0, now.Location())
	at := func(min int) time.Time { return time.Date(y, mo, d, min/60, min%60, 0, 0, now.Location()) }
	bed := t.WeekendUntil
	if schoolDay(midnight.AddDate(0, 0, 1).Weekday()) {
		bed = t.SchoolNightUntil
	}
	start, set := clock(t.From)
	if !set {
		start = 0
		if bed != "" {
			start = defaultFrom
		}
	}
	from = at(start)
	until = midnight.AddDate(0, 0, 1)
	if m, set := clock(bed); set {
		until = at(m)
	}
	return from, until, true
}

// Why a profile can't play right now.
const (
	ReasonAllowance = "allowance" // today's time is used up
	ReasonBedtime   = "bedtime"   // it's past the last hour
	ReasonEarly     = "early"     // it's before the first hour
)

// Usage is what a profile has had today: time played, extra time a parent
// gave, and until when a parent let it go on past the hours.
type Usage struct {
	UsedSec    int
	ExtraSec   int
	ExtraUntil time.Time
}

// Verdict is the answer for one moment.
type Verdict struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
	// AllowanceSec is today's allowance with any extra time added, -1 for none.
	// RemainingSec is how long playing may go on from now, whichever limit
	// comes first; -1 = nothing stops it today.
	AllowanceSec int `json:"allowanceSec"`
	UsedSec      int `json:"usedSec"`
	ExtraSec     int `json:"extraSec"`
	RemainingSec int `json:"remainingSec"`
	// Until is when playing must stop, and NextAt when it may start again.
	// Zero when there is no such moment today.
	Until  time.Time `json:"-"`
	NextAt time.Time `json:"-"`
}

// Check says whether the profile may play at now, having used what it has.
// Extra time counts twice over, as a parent means it to: it is added to the
// day's allowance, and until it runs out the hours don't apply.
func (t *TimeRules) Check(now time.Time, u Usage) Verdict {
	v := Verdict{Allowed: true, AllowanceSec: -1, UsedSec: u.UsedSec, ExtraSec: u.ExtraSec, RemainingSec: -1}
	if t.Empty() {
		return v
	}
	if a := t.AllowanceSec(now); a >= 0 {
		v.AllowanceSec = a + u.ExtraSec
		left := v.AllowanceSec - u.UsedSec
		if left <= 0 {
			v.Allowed, v.Reason, v.RemainingSec = false, ReasonAllowance, 0
			y, mo, d := now.Date()
			v.NextAt = time.Date(y, mo, d+1, 0, 0, 0, 0, now.Location())
			if from, _, ok := t.window(v.NextAt); ok {
				v.NextAt = from
			}
			return v
		}
		v.RemainingSec = left
		v.Until = now.Add(time.Duration(left) * time.Second)
	}
	from, until, ok := t.window(now)
	if !ok {
		return v
	}
	past := u.ExtraUntil.After(now) // a parent said: a little longer
	switch {
	case now.Before(from) && !past:
		v.Allowed, v.Reason, v.RemainingSec, v.NextAt, v.Until = false, ReasonEarly, 0, from, time.Time{}
		return v
	case !now.Before(until) && !past:
		v.Allowed, v.Reason, v.RemainingSec, v.Until = false, ReasonBedtime, 0, time.Time{}
		next, _, _ := t.window(now.AddDate(0, 0, 1))
		v.NextAt = next
		return v
	}
	stop := until
	if past && (u.ExtraUntil.After(until) || u.ExtraUntil.Before(from)) {
		stop = u.ExtraUntil
	}
	if left := int(stop.Sub(now) / time.Second); v.RemainingSec < 0 || left < v.RemainingSec {
		v.RemainingSec = max(left, 1)
		v.Until = stop
	}
	return v
}

// Message is what the person watching is told when they can't play.
func (v Verdict) Message() string {
	switch v.Reason {
	case ReasonAllowance:
		return "That's all the screen time for today. A parent can add more."
	case ReasonBedtime:
		return "It's past the time this profile can watch tonight. A parent can allow a little longer."
	case ReasonEarly:
		return "It's too early for this profile to watch. A parent can allow it."
	}
	return ""
}

// DayNumber is a day of the server's calendar as YYYYMMDD, as the watch
// history writes it.
func DayNumber(t time.Time) int {
	y, m, d := t.Date()
	return y*10000 + int(m)*100 + d
}
