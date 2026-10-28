package dubi

import (
	"context"
	"sort"
	"strconv"

	. "github.com/dubi-project/dubi-go/base"
)

const (
	START = "start"
	STOP  = "stop"
)

// Trigger listens for a process-local notification. Multiple registrations may
// use the same logical name; Profile selection applies to that name.
// Action is a notification callback with no return value. It may record a
// failure through ctx.Result for the process dispatcher to log.
type Trigger struct {
	Name     string
	Desc     string
	Nullable bool
	Args     Vars
	Data     Vars
	Settings Map
	Action   func(*Context)
}

func (Trigger) Scope() string { return "core:trigger" }

func isTrigger(value any) bool {
	switch value.(type) {
	case Trigger, *Trigger:
		return true
	default:
		return false
	}
}

func (c *coreModule) registerTrigger(name string, definition Trigger) {
	if definition.Name == "" {
		definition.Name = name
	}
	definition.Args = cloneTriggerVars(definition.Args, false)
	definition.Data = cloneTriggerVars(definition.Data, false)
	definition.Settings = cloneConfigMap(definition.Settings)
	c.triggers[name] = append(c.triggers[name], definition)
}

func cloneTriggerVars(fields Vars, optional bool) Vars {
	fields = fields.Clone()
	for name, field := range fields {
		field.Default = cloneConfigValue(field.Default)
		field.Settings = cloneConfigMap(field.Settings)
		field.Options = cloneConfigMap(field.Options)
		field.Children = cloneTriggerVars(field.Children, optional)
		if optional {
			field.Required = false
			field.Nullable = true
			field.Default = nil
		}
		fields[name] = field
	}
	return fields
}

func (c *coreModule) setupTriggers() {
	triggers.shutdown()
	for _, name := range c.triggerMethods {
		delete(c.methods, name)
	}
	c.triggerMethods = nil
	names := make([]string, 0, len(c.triggers))
	for name := range c.triggers {
		names = append(names, name)
	}
	sort.Strings(names)
	methods := make(map[string][]string, len(names))
	for _, name := range names {
		for _, definition := range c.triggers[name] {
			var internal string
			for {
				c.triggerSequence++
				internal = "_.trigger." + name + "." + strconv.FormatUint(c.triggerSequence, 10)
				_, methodExists := c.methods[internal]
				_, serviceExists := c.services[internal]
				if !methodExists && !serviceExists {
					break
				}
			}
			c.methods[internal] = Method{
				Name: definition.Name, Desc: definition.Desc,
				Args: cloneTriggerVars(definition.Args, definition.Nullable), Data: definition.Data,
				Settings: definition.Settings,
				Action: func(ctx *Context) (Map, Res) {
					ctx.Name, ctx.Config = name, definition
					if definition.Action == nil {
						return nil, OK
					}
					definition.Action(ctx)
					return nil, ctx.Result()
				},
			}
			c.triggerMethods = append(c.triggerMethods, internal)
			methods[name] = append(methods[name], internal)
		}
	}
	triggers.mu.Lock()
	triggers.ctx, triggers.cancel = context.WithCancel(context.Background())
	triggers.methods, triggers.accepting = methods, true
	triggers.mu.Unlock()
}
