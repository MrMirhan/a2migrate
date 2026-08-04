package platform

import "time"

// timeLike is an internal alias for time.Time so tests can swap the clock.
type timeLike = time.Time

func timeNow() timeLike { return time.Now() }
