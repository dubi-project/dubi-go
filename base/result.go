package base

// Res is the result contract shared by business methods and entry modules.
//
// The concrete implementation belongs to the dubi package. Keeping this as
// an interface lets the dependency-free base package avoid depending on the
// status and language catalogs owned by dubi.
type Res interface {
	OK() bool
	Fail() bool
	Code() int
	Status() string
	Args() []Any
	With(...Any) Res
	// Retry returns a copy with retry enabled (or the supplied flag). It does
	// not change the business code, status, or text arguments.
	Retry(...bool) Res
	Retriable() bool
	Error() string
}
