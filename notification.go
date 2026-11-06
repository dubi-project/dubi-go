package dubi

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	. "github.com/dubi-project/dubi-go/base"
)

// Notification describes a notification contract, never executable business
// logic. Definitions live outside Registry and Profile in a process-wide catalog.
type Notification struct {
	Name string
	Desc string
	Args Vars
}

var ErrNotificationNotFound = errors.New("dubi: notification not registered")

type notificationDefinition struct {
	value Notification
	plan  *MappingPlan
}

var notifications struct {
	sync.RWMutex
	items map[string]notificationDefinition
}

func registerNotification(name string, value any) bool {
	var definition Notification
	switch v := value.(type) {
	case Notification:
		definition = v
	case *Notification:
		if v == nil {
			panic("dubi: cannot register nil notification")
		}
		definition = *v
	default:
		return false
	}
	definition.Args = cloneTriggerVars(definition.Args, false)
	item := notificationDefinition{value: definition}
	if len(definition.Args) > 0 {
		item.plan = &MappingPlan{plan: compileMappingPlan(definition.Args)}
	}
	notifications.Lock()
	defer notifications.Unlock()
	if notifications.items == nil {
		notifications.items = make(map[string]notificationDefinition)
	}
	notifications.items[name] = item
	return true
}

// Notifications returns detached definitions keyed by registration name for
// documentation and introspection, including definitions outside the Profile.
func Notifications() map[string]Notification {
	notifications.RLock()
	defer notifications.RUnlock()
	items := make(map[string]Notification, len(notifications.items))
	for name, item := range notifications.items {
		value := item.value
		value.Args = cloneTriggerVars(value.Args, false)
		items[name] = value
	}
	return items
}

// Notify validates a registered notification before submitting it to BUS.
// Each listener group receives one delivery; accepted notifications are neither
// persisted nor retried. The returned error is not a listener business result.
func Notify(name string, values ...Map) error { return notifyBus(nil, name, busValue(values)) }

func (m *Meta) Notify(name string, values ...Map) error { return notifyBus(m, name, busValue(values)) }

func notifyBus(meta *Meta, name string, value Map) error {
	name = strings.TrimSpace(name)
	notifications.RLock()
	definition, exists := notifications.items[name]
	notifications.RUnlock()
	if !exists {
		return fmt.Errorf("%w: %s", ErrNotificationNotFound, name)
	}
	hook, snapshot, done, err := bus.acquire(meta)
	if err != nil {
		return err
	}
	defer done()
	value = cloneConfigMap(value)
	if definition.plan != nil {
		mapped, result := snapshot.ApplyMapping(definition.plan, value)
		if result != nil && result.Fail() {
			return result
		}
		// Mapping defaults may contain maps/slices from the definition. A Hook
		// must never be able to mutate the global contract through its payload.
		value = cloneConfigMap(mapped)
	}
	return hook.Notify(snapshot, name, value)
}
