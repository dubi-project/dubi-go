package data

import (
	"fmt"

	. "github.com/dubi-project/dubi-go/base"
)

// configuredSettings accepts the old singular configuration spelling while
// exposing only Settings in the Go API. Plural fields win on conflicts.
func configuredSettings(config Map) (Map, bool, error) {
	var result Map
	present := false
	for _, key := range []string{"setting", "settings"} {
		value, exists := config[key]
		if !exists {
			continue
		}
		present = true
		var fields Map
		switch item := value.(type) {
		case Map:
			fields = item
		case map[string]any:
			fields = Map(item)
		default:
			return nil, true, fmt.Errorf("%s must be an object", key)
		}
		result = mergeMapDefaults(fields, result)
	}
	return result, present, nil
}
