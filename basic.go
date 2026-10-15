package dubi

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"

	base "github.com/dubi-project/dubi-go/base"
)

// Var and Vars live in base so application code can share parameter
// definitions across HTTP and non-HTTP entry modules.

// Type describes a named value type used by Mapping.
type Type struct {
	Name  string
	Desc  string
	Alias []string
	Valid func(base.Any, base.Var) bool
	Value func(base.Any, base.Var) base.Any
}

// Status is the internal code assigned to one result status key.
type Status int

func (Status) Scope() string { return "basic:status" }

// Statuses is the status key to internal code catalog.
type Statuses map[string]Status

func (Statuses) Scope() string { return "basic:status" }

func (Type) Scope() string { return "basic:type" }

// Language contains a language's accepted request tags and translated text.
type Language struct {
	Name    string
	Desc    string
	Accepts []string
	Strings base.Map
}

func (Language) Scope() string { return "basic:language" }

// Mime maps a file extension to one or more MIME values.
type Mime []string

func (Mime) Scope() string { return "basic:mime" }

type basicModule struct {
	types     map[string]Type
	statuses  Statuses
	languages map[string]Language
	mimes     map[string]Mime
}

var basic = &basicModule{
	types:     make(map[string]Type),
	statuses:  make(Statuses),
	languages: make(map[string]Language),
	mimes:     make(map[string]Mime),
}

func init() {
	for name, config := range builtinTypes() {
		basic.types[name] = config
	}
	for name, values := range builtinMimes() {
		basic.mimes[name] = values
	}
	statusCatalog.RLock()
	for name, code := range statusCatalog.statuses {
		basic.statuses[name] = code
	}
	if len(statusCatalog.texts) > 0 {
		strings := base.Map{}
		for name, text := range statusCatalog.texts {
			strings[name] = text
		}
		basic.languages["default"] = Language{Name: "default", Strings: strings}
	}
	statusCatalog.RUnlock()
	Mount(basic)
}

var statusCatalog = struct {
	sync.RWMutex
	statuses Statuses
	texts    map[string]string
}{
	statuses: make(Statuses),
	texts:    make(map[string]string),
}

func (b *basicModule) Register(name string, value any) {
	switch item := value.(type) {
	case Type:
		b.registerType(name, item)
	case *Type:
		if item != nil {
			b.registerType(name, *item)
		}
	case Language:
		b.languages[name] = item
	case *Language:
		if item != nil {
			b.languages[name] = *item
		}
	case Mime:
		b.mimes[name] = append(Mime(nil), item...)
	case []string:
		b.mimes[name] = append(Mime(nil), item...)
	case Status:
		b.statuses[name] = item
	case Statuses:
		for key, code := range item {
			b.statuses[key] = code
		}
	}
}

func (b *basicModule) Reset() {
	b.types = builtinTypes()
	b.statuses = make(Statuses)
	b.languages = make(map[string]Language)
	b.mimes = builtinMimes()
	statusCatalog.RLock()
	for name, code := range statusCatalog.statuses {
		b.statuses[name] = code
	}
	for name, text := range statusCatalog.texts {
		if b.languages["default"].Strings == nil {
			b.languages["default"] = Language{Name: "default", Strings: base.Map{}}
		}
		b.languages["default"].Strings[name] = text
	}
	statusCatalog.RUnlock()
}

func (b *basicModule) registerType(name string, item Type) {
	if item.Name == "" {
		item.Name = name
	}
	keys := append([]string{name}, item.Alias...)
	for _, key := range keys {
		if strings.TrimSpace(key) != "" {
			b.types[strings.ToLower(strings.TrimSpace(key))] = item
		}
	}
}

func (*basicModule) Config(base.Map) error { return nil }
func (*basicModule) Setup(*Context) error  { return nil }
func (*basicModule) Open(*Context) error   { return nil }
func (*basicModule) Start(*Context) error  { return nil }
func (*basicModule) Stop(*Context) error   { return nil }
func (*basicModule) Close(*Context) error  { return nil }

// Mapping validates and converts data according to base.Vars.
func (rt *Runtime) Mapping(config base.Vars, data base.Map, values ...base.Map) (base.Map, base.Res) {
	out := base.Map{}
	if len(values) > 0 && values[0] != nil {
		out = values[0]
	}
	instance := rt.basicModule()
	return instance.mapping(config, data, out)
}

// MappingPlan is an immutable compiled form of a dynamic Vars definition.
// It is safe to reuse for concurrent requests as long as the original Vars
// definition is not mutated after PrepareMapping returns.
type MappingPlan struct {
	plan *mappingPlan
}

// PrepareMapping compiles a dynamic mapping definition for repeated use.
// Mapping remains available for one-off definitions and keeps the flexible
// map-based API used by application code.
func (rt *Runtime) PrepareMapping(config base.Vars) *MappingPlan {
	return &MappingPlan{plan: compileMappingPlan(config)}
}

// ApplyMapping applies a prepared mapping plan. A nil plan falls back to the
// ordinary dynamic Mapping path so callers can keep optional definitions
// simple.
func (rt *Runtime) ApplyMapping(plan *MappingPlan, data base.Map, values ...base.Map) (base.Map, base.Res) {
	if plan == nil || plan.plan == nil {
		return rt.Mapping(nil, data, values...)
	}
	out := base.Map{}
	if len(values) > 0 && values[0] != nil {
		out = values[0]
	}
	return rt.basicModule().mappingPlan(plan.plan, data, out, "")
}

// Mapping uses the default Runtime's basic type registry.
func Mapping(config base.Vars, data base.Map, values ...base.Map) (base.Map, base.Res) {
	return DefaultRuntime().Mapping(config, data, values...)
}

// PrepareMapping compiles a dynamic mapping definition using the process
// registry. Mapping plans are safe to reuse for concurrent requests.
func PrepareMapping(config base.Vars) *MappingPlan {
	return DefaultRuntime().PrepareMapping(config)
}

// ApplyMapping applies a prepared process-level mapping plan.
func ApplyMapping(plan *MappingPlan, data base.Map, values ...base.Map) (base.Map, base.Res) {
	return DefaultRuntime().ApplyMapping(plan, data, values...)
}

func (b *basicModule) mapping(config base.Vars, data base.Map, out base.Map) (base.Map, base.Res) {
	return b.mappingPlan(compileMappingPlan(config), data, out, "")
}

// mappingPlan is compiled once for one Mapping call. Vars is intentionally a
// map for the public dynamic API, so compiling the sorted keys once avoids
// repeating the sort for every element of a nested array.
type mappingPlan struct {
	fields []mappingField
}

type mappingField struct {
	name       string
	definition base.Var
	children   *mappingPlan
}

func compileMappingPlan(config base.Vars) *mappingPlan {
	keys := mappingKeys(config)
	plan := &mappingPlan{fields: make([]mappingField, 0, len(keys))}
	for _, key := range keys {
		definition := config[key]
		field := mappingField{name: key, definition: definition}
		if len(definition.Children) > 0 {
			field.children = compileMappingPlan(definition.Children)
		}
		plan.fields = append(plan.fields, field)
	}
	return plan
}

func (b *basicModule) mappingPlan(plan *mappingPlan, data base.Map, out base.Map, prefix string) (base.Map, base.Res) {
	if data == nil {
		data = base.Map{}
	}
	for _, field := range plan.fields {
		key := field.name
		definition := field.definition
		path := mappingPath(prefix, key)
		if prefix == "" && strings.TrimSpace(definition.Name) != "" {
			path = strings.TrimSpace(definition.Name)
		}
		value, found := lookupValue(data, key, definition.Alias)
		if !found {
			if definition.Default != nil {
				value, found = resolveMappingDefault(definition.Default), true
			} else if definition.Required && !definition.Nullable {
				return out, mappingMissing(definition, path)
			} else if definition.Nullable {
				continue
			} else {
				continue
			}
		}
		if definition.Default != nil && isEmptyBasic(value) {
			value = resolveMappingDefault(definition.Default)
		}
		if definition.Required && !definition.Nullable && definition.Default == nil && isEmptyBasic(value) {
			return out, mappingMissing(definition, path)
		}
		if value == nil && definition.Nullable {
			out[key] = nil
			continue
		}
		if isNilMappingValue(value) {
			if definition.Required && !definition.Nullable {
				return out, mappingMissing(definition, path)
			}
			out[key] = nil
			continue
		}
		decoded := false
		if definition.Decode != "" {
			decodedValue, err := b.decode(definition.Decode, value)
			if err != nil {
				return out, mappingError(definition.Error, 400, mappingValueError(path, err.Error()))
			}
			value = decodedValue
			decoded = true
		}
		if value == nil {
			if definition.Required && !definition.Nullable {
				return out, mappingMissing(definition, path)
			}
			out[key] = nil
			continue
		}
		if len(definition.Children) > 0 {
			if isStructuredMappingType(definition.Type) && isTextMappingValue(value) {
				decoded, err := b.decode("json", value)
				if err != nil {
					return out, mappingError(definition.Error, 400, mappingValueError(path, err.Error()))
				}
				value = decoded
			}
			converted, result := b.mapChildren(field.children, value, path, definition.Error)
			if result != nil && result.Fail() {
				return out, result
			}
			value = converted
		}
		converted, err := b.convert(definition.Type, value, definition)
		if err != nil {
			return out, mappingError(definition.Error, 400, mappingValueError(path, err.Error()))
		}
		if definition.Valid != nil && !definition.Valid(converted, definition) {
			return out, mappingError(definition.Error, 400, "invalid argument: "+path)
		}
		if definition.Value != nil {
			converted = definition.Value(converted, definition)
		}
		if definition.Required && !definition.Nullable && definition.Default == nil && isEmptyBasic(converted) {
			return out, mappingMissing(definition, path)
		}
		if isNilMappingValue(converted) {
			if definition.Required && !definition.Nullable {
				return out, mappingMissing(definition, path)
			}
			out[key] = nil
			continue
		}
		if definition.Encode != "" && !decoded {
			encoded, err := b.encode(definition.Encode, converted)
			if err != nil {
				return out, mappingError(definition.Error, 400, mappingValueError(path, err.Error()))
			}
			converted = encoded
		}
		out[key] = converted
	}
	return out, nil
}

func mappingPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

func mappingValueError(path, message string) string {
	if path == "" {
		return message
	}
	return path + ": " + message
}

func resolveMappingDefault(value any) any {
	if value == nil {
		return nil
	}
	raw := reflect.ValueOf(value)
	if raw.Kind() != reflect.Func || raw.IsNil() || raw.Type().NumIn() != 0 || raw.Type().NumOut() != 1 {
		return value
	}
	return raw.Call(nil)[0].Interface()
}

func mappingKeys(config base.Vars) []string {
	keys := make([]string, 0, len(config))
	for key := range config {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func isEmptyBasic(value any) bool {
	if isNilMappingValue(value) {
		return true
	}
	switch item := value.(type) {
	case string:
		return strings.TrimSpace(item) == ""
	case []byte:
		return len(item) == 0
	}
	raw := reflect.ValueOf(value)
	switch raw.Kind() {
	case reflect.Map, reflect.Slice, reflect.Array:
		return raw.Len() == 0
	}
	return false
}

func isNilMappingValue(value any) bool {
	if value == nil {
		return true
	}
	raw := reflect.ValueOf(value)
	switch raw.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return raw.IsNil()
	default:
		return false
	}
}

func isTextMappingValue(value any) bool {
	switch value.(type) {
	case string, []byte:
		return true
	default:
		return false
	}
}

func isStructuredMappingType(typeName string) bool {
	typeName = strings.ToLower(strings.TrimSpace(typeName))
	if typeName == "json" || typeName == "jsonb" || typeName == "object" || typeName == "map" {
		return true
	}
	_, ok := mappingArrayElementType(typeName)
	return ok
}

func (b *basicModule) mapChildren(plan *mappingPlan, value any, path string, custom any) (any, base.Res) {
	raw := reflect.ValueOf(value)
	if raw.IsValid() && (raw.Kind() == reflect.Slice || raw.Kind() == reflect.Array) {
		result := make([]any, 0, raw.Len())
		for index := 0; index < raw.Len(); index++ {
			item, ok := asMapValue(raw.Index(index).Interface())
			if !ok {
				return nil, mappingError(custom, 400, mappingValueError(path+"["+strconv.Itoa(index)+"]", "expected object"))
			}
			mapped, res := b.mappingPlan(plan, item, base.Map{}, path+"["+strconv.Itoa(index)+"]")
			if res != nil && res.Fail() {
				return nil, res
			}
			result = append(result, mapped)
		}
		return result, nil
	}
	item, ok := asMapValue(value)
	if !ok {
		return nil, mappingError(custom, 400, mappingValueError(path, "expected object"))
	}
	mapped, res := b.mappingPlan(plan, item, base.Map{}, path)
	return mapped, res
}

func asMapValue(value any) (base.Map, bool) {
	switch item := value.(type) {
	case base.Map:
		return item, true
	case map[string]any:
		return base.Map(item), true
	}
	raw := reflect.ValueOf(value)
	if !raw.IsValid() || raw.Kind() != reflect.Map || raw.Type().Key().Kind() != reflect.String {
		return nil, false
	}
	result := base.Map{}
	iterator := raw.MapRange()
	for iterator.Next() {
		result[iterator.Key().String()] = iterator.Value().Interface()
	}
	return result, true
}

func lookupValue(data base.Map, name string, aliases []string) (any, bool) {
	if value, ok := data[name]; ok {
		return value, true
	}
	for _, alias := range aliases {
		if value, ok := data[alias]; ok {
			return value, true
		}
	}
	return nil, false
}

func (b *basicModule) convert(typeName string, value any, definition base.Var) (any, error) {
	if definition.Value != nil {
		return value, nil
	}
	typeName = strings.ToLower(strings.TrimSpace(typeName))
	if typeName == "" || typeName == "any" {
		return value, nil
	}
	if elementType, ok := mappingArrayElementType(typeName); ok {
		if text, ok := value.(string); ok {
			decoded, err := b.decode("json", text)
			if err != nil {
				return nil, err
			}
			value = decoded
		} else if bytes, ok := value.([]byte); ok {
			decoded, err := b.decode("json", bytes)
			if err != nil {
				return nil, err
			}
			value = decoded
		}
		raw := reflect.ValueOf(value)
		if !raw.IsValid() || (raw.Kind() != reflect.Slice && raw.Kind() != reflect.Array) {
			return nil, fmt.Errorf("expected array")
		}
		converted := make([]any, raw.Len())
		for index := 0; index < raw.Len(); index++ {
			var err error
			converted[index], err = b.convert(elementType, raw.Index(index).Interface(), base.Var{})
			if err != nil {
				return nil, err
			}
		}
		return converted, nil
	}
	if custom, ok := b.types[typeName]; ok && (custom.Valid != nil || custom.Value != nil) {
		if custom.Valid != nil && !custom.Valid(value, definition) {
			return nil, fmt.Errorf("invalid value")
		}
		if custom.Value != nil {
			return custom.Value(value, definition), nil
		}
		return value, nil
	}
	switch typeName {
	case "string", "text":
		if bytes, ok := value.([]byte); ok {
			return string(bytes), nil
		}
		return fmt.Sprint(value), nil
	case "bool", "boolean":
		return strconv.ParseBool(fmt.Sprint(value))
	case "int", "int8", "int16", "int32", "int64":
		bits := 64
		if len(typeName) > 3 {
			bits, _ = strconv.Atoi(typeName[3:])
		}
		return strconv.ParseInt(fmt.Sprint(value), 10, bits)
	case "uint", "uint8", "uint16", "uint32", "uint64":
		bits := 64
		if len(typeName) > 4 {
			bits, _ = strconv.Atoi(typeName[4:])
		}
		return strconv.ParseUint(fmt.Sprint(value), 10, bits)
	case "float", "float32", "float64":
		bits := 64
		if len(typeName) > 5 {
			bits, _ = strconv.Atoi(typeName[5:])
		}
		return strconv.ParseFloat(fmt.Sprint(value), bits)
	case "json", "jsonb", "object", "map":
		if _, ok := value.(string); ok {
			return b.decode("json", value)
		}
		if _, ok := value.([]byte); ok {
			return b.decode("json", value)
		}
		return value, nil
	}
	return value, nil
}

func mappingArrayElementType(typeName string) (string, bool) {
	if typeName == "array" {
		return "any", true
	}
	if strings.HasPrefix(typeName, "[]") {
		return strings.TrimPrefix(typeName, "[]"), true
	}
	if strings.HasPrefix(typeName, "[") && strings.HasSuffix(typeName, "]") {
		return strings.TrimSuffix(strings.TrimPrefix(typeName, "["), "]"), true
	}
	return "", false
}

func (b *basicModule) decode(name string, value any) (any, error) {
	return codec.decode(name, value, nil)
}

func (b *basicModule) encode(name string, value any) (any, error) {
	return codec.encode(name, value)
}

func mappingMissing(definition base.Var, path string) base.Res {
	custom := definition.Empty
	if custom == nil {
		custom = definition.Error
	}
	return mappingError(custom, 400, "missing argument: "+path)
}

func mappingError(custom any, code int, fallback string) base.Res {
	switch value := custom.(type) {
	case base.Res:
		return value
	case error:
		return ErrorResult(value)
	case string:
		return ResultOf(code, value)
	default:
		return ResultOf(code, fallback)
	}
}

func normalizeBasicValue(value any) any {
	switch item := value.(type) {
	case map[string]any:
		result := base.Map{}
		for key, value := range item {
			result[key] = normalizeBasicValue(value)
		}
		return result
	case []any:
		result := make([]any, len(item))
		for index, value := range item {
			result[index] = normalizeBasicValue(value)
		}
		return result
	default:
		return value
	}
}

func (rt *Runtime) basicModule() *basicModule {
	return basic
}

func processBasic() *basicModule { return basic }

func builtinTypes() map[string]Type {
	return map[string]Type{
		"any": {}, "string": {}, "text": {}, "bool": {}, "boolean": {},
		"int": {}, "int8": {}, "int16": {}, "int32": {}, "int64": {},
		"uint": {}, "uint8": {}, "uint16": {}, "uint32": {}, "uint64": {},
		"float": {}, "float32": {}, "float64": {}, "json": {},
	}
}

func builtinMimes() map[string]Mime {
	return map[string]Mime{
		"json": {"application/json"}, "html": {"text/html; charset=utf-8"},
		"txt": {"text/plain; charset=utf-8"}, "js": {"application/javascript"},
		"css": {"text/css"}, "xml": {"application/xml"},
	}
}

func (rt *Runtime) Mimetype(ext string, defaults ...string) string {
	b := rt.basicModule()
	mimeExt := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(ext)), ".")
	if strings.Contains(ext, "/") {
		return ext
	}
	if values, ok := b.mimes[mimeExt]; ok && len(values) > 0 {
		return values[0]
	}
	if values, ok := b.mimes["*"]; ok && len(values) > 0 {
		return values[0]
	}
	if len(defaults) > 0 {
		return defaults[0]
	}
	return "application/octet-stream"
}

func Mimetype(ext string, defaults ...string) string {
	return DefaultRuntime().Mimetype(ext, defaults...)
}

func registerStatus(key string, code Status, text string) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	statusCatalog.Lock()
	statusCatalog.statuses[key] = code
	if text != "" {
		statusCatalog.texts[strings.ReplaceAll(key, ".", "_")] = text
	}
	statusCatalog.Unlock()
	if basic != nil {
		basic.statuses[key] = code
		if text != "" {
			language := basic.languages["default"]
			if language.Strings == nil {
				language.Strings = base.Map{}
			}
			language.Strings[strings.ReplaceAll(key, ".", "_")] = text
			basic.languages["default"] = language
		}
	}
}

// StatusCode returns the internal code registered for a status key.
func (rt *Runtime) StatusCode(key string, defaults ...int) int {
	if code, ok := rt.basicModule().statuses[key]; ok {
		return int(code)
	}
	if len(defaults) > 0 {
		return defaults[0]
	}
	return -1
}

func StatusCode(key string, defaults ...int) int {
	return DefaultRuntime().StatusCode(key, defaults...)
}

// Languages returns the Runtime's registered languages.
func (rt *Runtime) Languages() map[string]Language {
	result := map[string]Language{}
	for name, language := range rt.basicModule().languages {
		copyOf := language
		copyOf.Accepts = append([]string(nil), language.Accepts...)
		copyOf.Strings = language.Strings.Clone()
		result[name] = copyOf
	}
	return result
}

// DetectLanguage selects the first registered language accepted by a header.
func (rt *Runtime) DetectLanguage(header string, fallback ...string) string {
	if strings.TrimSpace(header) == "" {
		if len(fallback) > 0 {
			return fallback[0]
		}
		return ""
	}
	b := rt.basicModule()
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		if part == "" {
			continue
		}
		for name, language := range b.languages {
			if strings.EqualFold(name, part) {
				return name
			}
			for _, accepted := range language.Accepts {
				if strings.EqualFold(accepted, part) || strings.HasPrefix(strings.ToLower(part), strings.ToLower(accepted)+"-") {
					return name
				}
			}
		}
	}
	if len(fallback) > 0 {
		return fallback[0]
	}
	return ""
}

// String resolves one registered translation key.
func (rt *Runtime) String(language, key string, args ...base.Any) string {
	return rt.basicModule().text(language, key, args...)
}

func (b *basicModule) text(language, key string, args ...base.Any) string {
	key = strings.ReplaceAll(key, ".", "_")
	text := key
	if config, ok := b.languages[language]; ok {
		if translated, exists := config.Strings[key]; exists {
			if value, ok := translated.(string); ok {
				text = value
			}
		}
	} else if config, ok := b.languages["default"]; ok {
		if translated, exists := config.Strings[key]; exists {
			if value, ok := translated.(string); ok {
				text = value
			}
		}
	}
	if len(args) > 0 && strings.Contains(text, "%") {
		return fmt.Sprintf(text, args...)
	}
	return text
}

func String(language, key string, args ...base.Any) string {
	return DefaultRuntime().String(language, key, args...)
}

// DetectLanguage resolves a request language using the process language
// catalog.
func DetectLanguage(header string, fallback ...string) string {
	return DefaultRuntime().DetectLanguage(header, fallback...)
}
