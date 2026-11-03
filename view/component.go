package view

import (
	"time"

	. "github.com/dubi-project/dubi-go/base"
)

// Helper registers a template function. Action uses html/template's function
// signatures: one result, or a result followed by error.
type Helper struct {
	Name   string
	Desc   string
	Alias  []string
	Action any
}

func (Helper) Scope() string    { return "view:helper" }
func (Helper) Foundation() bool { return true }

// Body separates shared template data from the current page's model. Helpers
// override registered functions for this render, except reserved built-ins.
type Body struct {
	View     string
	Site     string
	Language string
	Timezone *time.Location
	Data     Map
	Model    Map
	Helpers  Map
}
