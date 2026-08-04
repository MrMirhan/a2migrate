package sync

import "time"

// timeT is an alias so tests can stub clocks via time package without
// aliasing everywhere.
type timeT = time.Time