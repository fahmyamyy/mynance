package timeutil

import "time"

// Now returns current UTC time. Use for Go-side timestamps.
// Prefer DB-side now() for columns with DEFAULT now().
func Now() time.Time {
	return time.Now().UTC()
}
