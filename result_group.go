package dubi

import (
	"strings"

	. "github.com/dubi-project/dubi-go/base"
)

// ResultGroup creates a group of result definitions with a shared status prefix.
// Failure codes start at 1000, or the supplied base. Define results during
// package initialization; numbering follows definition order within each group.
func ResultGroup(name string, bases ...int) *resultGroup {
	base := 1000
	if len(bases) > 0 {
		base = bases[0]
	}
	return &resultGroup{name: name, base: base}
}

type resultGroup struct {
	name string
	base int
}

func (g *resultGroup) Name() string { return g.name }

// Result defines and registers a result using the group's status prefix.
// Success always has code zero and does not advance the failure sequence.
// Codes are local to this group; different groups may use the same codes.
func (g *resultGroup) Result(ok bool, status, text string) Res {
	code := 0
	if !ok {
		code = g.base
		g.base++
	}
	if g.name != "" && !strings.HasPrefix(status, g.name+".") {
		status = g.name + "." + status
	}
	return Result(code, status, text)
}
