// Package quiet knows the night, when nothing may make a phone sound: no
// SMS goes out, and a push notification arrives without sound or
// vibration.
package quiet

import (
	"time"

	// The image starts from scratch and holds no time zone files, so the
	// binary carries them.
	_ "time/tzdata"
)

const (
	// From is the hour in the time zone Zone when the night starts.
	From = 22

	// Until is the hour when the night ends.
	Until = 7

	// Zone is the time zone of the night. The people of the site live in
	// Denmark.
	Zone = "Europe/Copenhagen"
)

// zone is loaded once. The embedded data of time/tzdata always holds it,
// so a failure is a broken build, not a case to handle.
var zone = mustLoad(Zone)

// Night answers whether a moment falls in the night in Denmark.
func Night(at time.Time) bool {
	hour := at.In(zone).Hour()

	return hour >= From || hour < Until
}

// Location returns the time zone of the night.
func Location() *time.Location {
	return zone
}

func mustLoad(name string) *time.Location {
	location, err := time.LoadLocation(name)
	if err != nil {
		panic("quiet: the time zone " + name + " is missing: " + err.Error())
	}

	return location
}
