package dubi

// Foundation marks a registration as a process-wide resource. When it returns
// true, the registration is loaded regardless of Profile include or exclude.
type Foundation interface {
	Foundation() bool
}
