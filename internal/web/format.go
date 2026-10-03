// Package web renders the pages and the fragments of the application.
package web

import (
	"strings"
	"time"
)

// clock writes the time of day. A bubble uses it, because the date line above
// already says which day it is.
func clock(at time.Time) string {
	return at.Local().Format("15:04")
}

// fullTime writes the whole moment, which every bubble carries in its title.
func fullTime(at time.Time) string {
	return at.Local().Format("2 January 2006 at 15:04")
}

// splitName cuts a full name into its parts.
func splitName(name string) []string {
	return strings.Fields(name)
}
