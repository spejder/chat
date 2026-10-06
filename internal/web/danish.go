package web

import (
	"strconv"
	"time"
)

// The names of the months and the days in Danish. The time package of Go
// writes them in English only. Danish writes them in lower case.
var (
	danishMonths = [...]string{
		"januar", "februar", "marts", "april", "maj", "juni",
		"juli", "august", "september", "oktober", "november", "december",
	}

	danishShortMonths = [...]string{
		"jan.", "feb.", "mar.", "apr.", "maj", "jun.",
		"jul.", "aug.", "sep.", "okt.", "nov.", "dec.",
	}

	danishDays = [...]string{
		"søndag", "mandag", "tirsdag", "onsdag", "torsdag", "fredag", "lørdag",
	}
)

// longDate writes a date the Danish way, for example "6. oktober 2026".
func longDate(at time.Time) string {
	return strconv.Itoa(at.Day()) + ". " + danishMonths[at.Month()-1] + " " + strconv.Itoa(at.Year())
}

// shortDate writes a date without the year, for example "6. okt.".
func shortDate(at time.Time) string {
	return strconv.Itoa(at.Day()) + ". " + danishShortMonths[at.Month()-1]
}

// weekday writes the name of the day, for example "mandag".
func weekday(at time.Time) string {
	return danishDays[at.Weekday()]
}
