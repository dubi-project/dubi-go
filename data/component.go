package data

import . "github.com/dubi-project/dubi-go/base"

type (
	Index struct {
		Name   string
		Fields []string
		Unique bool
	}

	Cascade struct {
		Table      string
		ForeignKey string
	}

	Table struct {
		Name     string
		Desc     string
		Schema   string
		Table    string
		Key      string
		Fields   Vars
		Indexes  []Index
		Cascades []Cascade
		Settings Map
	}

	View struct {
		Name     string
		Desc     string
		Schema   string
		View     string
		Key      string
		Fields   Vars
		Settings Map
	}

	Model struct {
		Name     string
		Desc     string
		Schema   string
		Model    string
		Key      string
		Fields   Vars
		Settings Map
	}
)

func (Table) Scope() string { return "data:table" }
func (View) Scope() string  { return "data:view" }
func (Model) Scope() string { return "data:model" }

func (Table) Foundation() bool { return true }
func (View) Foundation() bool  { return true }
func (Model) Foundation() bool { return true }
