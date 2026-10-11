package dubi

import "fmt"

type NotFoundError struct {
	Kind string
	Name string
}

func (e *NotFoundError) Error() string { return fmt.Sprintf("dubi: %s %q not found", e.Kind, e.Name) }

type TypeError struct {
	Name string
	Want string
	Got  any
}

func (e *TypeError) Error() string {
	return fmt.Sprintf("dubi: registration %q is not a %s (got %T)", e.Name, e.Want, e.Got)
}

func (s lifecycle) String() string {
	switch s {
	case stateNew:
		return "new"
	case stateSetup:
		return "setup"
	case stateOpen:
		return "open"
	case stateStarted:
		return "started"
	case stateStopped:
		return "stopped"
	case stateClosed:
		return "closed"
	default:
		return "unknown"
	}
}
