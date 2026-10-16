package base

// HTTP method names use their canonical lower-case form. Router and request
// code normalize methods at the boundary, so these constants can be used in
// dynamic registrations without depending on net/http.
const (
	GET     = "get"
	POST    = "post"
	PUT     = "put"
	DELETE  = "delete"
	PATCH   = "patch"
	OPTIONS = "options"
	HEAD    = "head"
	CONNECT = "connect"
	TRACE   = "trace"
)
