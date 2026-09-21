// Package web renders the pages and the fragments of the application.
package web

import (
	"strings"
	"time"
)

// shortTime writes a time for a reader: the clock alone for today, the date
// and the clock otherwise.
func shortTime(at time.Time) string {
	now := time.Now()

	if at.Year() == now.Year() && at.YearDay() == now.YearDay() {
		return at.Local().Format("15:04")
	}

	return at.Local().Format("2006-01-02 15:04")
}

// clock writes the time of day. A bubble uses it, because the date line above
// already says which day it is.
func clock(at time.Time) string {
	return at.Local().Format("15:04")
}

// splitName cuts a full name into its parts.
func splitName(name string) []string {
	return strings.Fields(name)
}
