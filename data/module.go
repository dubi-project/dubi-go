package data

import (
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	dubi "github.com/dubi-project/dubi-go"
	. "github.com/dubi-project/dubi-go/base"
)

func init() {
	dubi.Mount(module)
}

var _ dubi.Module = (*Module)(nil)

var module = &Module{
	configs:        make(Configs, 0),
	fileConfigs:    make(map[string]bool),
	fileConfigKeys: make(map[string]map[string]struct{}),
	instances:      make(map[string]*Instance, 0),
	tables:         make(map[string]Table, 0),
	views:          make(map[string]View, 0),
	models:         make(map[string]Model, 0),
	migrations:     make(map[string]Migration, 0),
	watchers:       make(map[string]Watcher, 0),
	dispatchers:    make(map[string]*changeDispatcher, 0),
}

type (
	Module struct {
		mutex sync.RWMutex

		initialized bool
		connected   bool
		started     bool

		configs        Configs
		fileConfigs    map[string]bool
		fileConfigKeys map[string]map[string]struct{}
		instances      map[string]*Instance
		tables         map[string]Table
		views          map[string]View
		models         map[string]Model
		migrations     map[string]Migration
		watchers       map[string]Watcher
		dispatchers    map[string]*changeDispatcher
	}

	Configs map[string]Config
	Config  struct {
		Type        string
		Url         string
		Schema      string
		Mapping     bool
		MaxOpen     int
		MaxIdle     int
		MaxLifetime time.Duration
		MaxIdleTime time.Duration
		ReadOnly    bool
		Trash       TrashOptions
		Watcher     Map
		Migrate     MigrateOptions
		Setting     Map
	}

	Instance struct {
		conn Connection

		Name    string
		Config  Config
		Setting Map
	}
)

func (m *Module) Register(name string, value Any) {
	switch v := value.(type) {
	case Config:
		m.RegisterConfig(name, v)
	case Configs:
		m.RegisterConfigs(v)
	case Table:
		m.RegisterTable(name, v)
	case View:
		m.RegisterView(name, v)
	case Model:
		m.RegisterModel(name, v)
	case Migration:
		m.RegisterMigration(name, v)
	case Watcher:
		m.RegisterWatcher(name, v)
	case Watchers:
		m.RegisterWatchers(v)
	case InsertWatcher:
		m.RegisterInsertWatcher(name, v)
	case UpdateWatcher:
		m.RegisterUpdateWatcher(name, v)
	case UpsertWatcher:
		m.RegisterUpsertWatcher(name, v)
	case DeleteWatcher:
		m.RegisterDeleteWatcher(name, v)
	}
}

func (m *Module) RegisterConfig(name string, cfg Config) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if name == "" {
		name = defaultName
	}
	// Runtime configuration is authoritative. Code registrations provide
	// defaults only when the same connection was not configured externally.
	if m.fileConfigs[name] {
		m.configs[name] = mergeConfigDefaults(m.configs[name], cfg, m.fileConfigKeys[name])
		return
	}
	m.configs[name] = cfg
}

func (m *Module) RegisterConfigs(configs Configs) {
	for k, v := range configs {
		m.RegisterConfig(k, v)
	}
}

func (m *Module) RegisterTable(name string, table Table) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if name == "" {
		return
	}
	table.Name = name
	if table.Key == "" {
		table.Key = "id"
	}
	m.tables[name] = table
}

func (m *Module) RegisterView(name string, view View) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if name == "" {
		return
	}
	view.Name = name
	if view.Key == "" {
		view.Key = "id"
	}
	m.views[name] = view
}

func (m *Module) RegisterModel(name string, model Model) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if name == "" {
		return
	}
	model.Name = name
	if model.Key == "" {
		model.Key = "id"
	}
	m.models[name] = model
}

func (m *Module) Config(global Map) error {
	cfgAny, ok := global["data"]
	if !ok {
		return nil
	}
	cfgMap, ok := cfgAny.(Map)
	if !ok || cfgMap == nil {
		return nil
	}

	if connections, ok := cfgMap["connections"].(Map); ok {
		for name, value := range connections {
			if item, ok := value.(Map); ok {
				if err := m.configure(name, item); err != nil {
					return err
				}
			}
		}
		root := cloneConfigMapWithout(cfgMap, "connections")
		if len(root) > 0 {
			if err := m.configure(defaultName, root); err != nil {
				return err
			}
		}
		return nil
	}
	if isSingleConnectionConfig(cfgMap) {
		return m.configure(defaultName, cfgMap)
	}
	for name, value := range cfgMap {
		if item, ok := value.(Map); ok {
			if err := m.configure(name, item); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *Module) configure(name string, cfg Map) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.fileConfigs == nil {
		m.fileConfigs = make(map[string]bool)
	}
	if m.fileConfigKeys == nil {
		m.fileConfigKeys = make(map[string]map[string]struct{})
	}
	m.fileConfigs[name] = true
	m.fileConfigKeys[name] = configKeys(cfg)

	out := Config{}
	if vv, ok := m.configs[name]; ok {
		out = vv
	}

	for _, key := range []string{"type"} {
		if v, ok := cfg[key].(string); ok && v != "" {
			out.Type = v
			break
		}
	}
	if v, ok := cfg["url"].(string); ok {
		out.Url = v
	}
	var err error
	out.Url, err = configuredURL(cfg, out.Url)
	if err != nil {
		return err
	}
	if v, ok := cfg["schema"].(string); ok {
		out.Schema = v
	}
	if v, ok := cfg["mapping"]; ok {
		if vv, ok := parseBool(v); ok {
			out.Mapping = vv
		}
	}
	if v, ok := cfg["fieldMapping"]; ok {
		if vv, ok := parseBool(v); ok {
			out.Mapping = vv
		}
	}
	if v, ok := cfg["field_mapping"]; ok {
		if vv, ok := parseBool(v); ok {
			out.Mapping = vv
		}
	}
	if v, ok := cfg["readOnly"]; ok {
		if vv, ok := parseBool(v); ok {
			out.ReadOnly = vv
		}
	}
	if v, ok := cfg["readonly"]; ok {
		if vv, ok := parseBool(v); ok {
			out.ReadOnly = vv
		}
	}
	if v, ok := parseBool(cfg["trash"]); ok {
		out.Trash.Enable = v
	}
	switch v := cfg["trash"].(type) {
	case Map:
		if vv, ok := parseBool(v["enable"]); ok {
			out.Trash.Enable = vv
		}
		if vv, ok := v["field"].(string); ok {
			out.Trash.Field = strings.TrimSpace(vv)
		}
		if vv, ok := v["value"].(string); ok {
			out.Trash.Value = strings.TrimSpace(vv)
		}
		if vv, ok := v["cascade"].(string); ok {
			out.Trash.Cascade = strings.TrimSpace(vv)
		}
	}
	if v, ok := parseBool(cfg["trashEnable"]); ok {
		out.Trash.Enable = v
	}
	if v, ok := parseBool(cfg["trash_enable"]); ok {
		out.Trash.Enable = v
	}
	if v, ok := cfg["trashField"].(string); ok {
		out.Trash.Field = strings.TrimSpace(v)
	}
	if v, ok := cfg["trash_field"].(string); ok {
		out.Trash.Field = strings.TrimSpace(v)
	}
	if v, ok := cfg["trashValue"].(string); ok {
		out.Trash.Value = strings.TrimSpace(v)
	}
	if v, ok := cfg["trash_value"].(string); ok {
		out.Trash.Value = strings.TrimSpace(v)
	}
	if v, ok := cfg["trashCascade"].(string); ok {
		out.Trash.Cascade = strings.TrimSpace(v)
	}
	if v, ok := cfg["trash_cascade"].(string); ok {
		out.Trash.Cascade = strings.TrimSpace(v)
	}
	if v, ok := cfg["trashCascadeValue"].(string); ok {
		out.Trash.Cascade = strings.TrimSpace(v)
	}
	if v, ok := cfg["trash_cascade_value"].(string); ok {
		out.Trash.Cascade = strings.TrimSpace(v)
	}
	if v, ok := cfg["watcher"].(Map); ok {
		out.Watcher = v
	}
	if v, ok := parseIntAny(cfg["maxOpen"]); ok {
		out.MaxOpen = v
	}
	if v, ok := parseIntAny(cfg["max_open"]); ok {
		out.MaxOpen = v
	}
	if v, ok := parseIntAny(cfg["maxIdle"]); ok {
		out.MaxIdle = v
	}
	if v, ok := parseIntAny(cfg["max_idle"]); ok {
		out.MaxIdle = v
	}
	if v, ok := parseDurationAny(cfg["maxLifetime"]); ok {
		out.MaxLifetime = v
	}
	if v, ok := parseDurationAny(cfg["max_lifetime"]); ok {
		out.MaxLifetime = v
	}
	if v, ok := parseDurationAny(cfg["maxIdleTime"]); ok {
		out.MaxIdleTime = v
	}
	if v, ok := parseDurationAny(cfg["max_idle_time"]); ok {
		out.MaxIdleTime = v
	}
	if pool, ok := cfg["pool"].(Map); ok {
		if v, ok := parseIntAny(pool["maxOpen"]); ok {
			out.MaxOpen = v
		}
		if v, ok := parseIntAny(pool["max_open"]); ok {
			out.MaxOpen = v
		}
		if v, ok := parseIntAny(pool["maxIdle"]); ok {
			out.MaxIdle = v
		}
		if v, ok := parseIntAny(pool["max_idle"]); ok {
			out.MaxIdle = v
		}
		if v, ok := parseDurationAny(pool["maxLifetime"]); ok {
			out.MaxLifetime = v
		}
		if v, ok := parseDurationAny(pool["max_lifetime"]); ok {
			out.MaxLifetime = v
		}
		if v, ok := parseDurationAny(pool["maxIdleTime"]); ok {
			out.MaxIdleTime = v
		}
		if v, ok := parseDurationAny(pool["max_idle_time"]); ok {
			out.MaxIdleTime = v
		}
	}
	if v, ok := cfg["migrate"].(Map); ok {
		if vv, ok := v["startup"].(string); ok {
			out.Migrate.Startup = strings.ToLower(strings.TrimSpace(vv))
		}
		if vv, ok := v["start"].(string); ok {
			out.Migrate.Startup = strings.ToLower(strings.TrimSpace(vv))
		}
		if vv, ok := v["mode"].(string); ok {
			out.Migrate.Mode = strings.ToLower(strings.TrimSpace(vv))
		}
		if vv, ok := parseBool(v["dryRun"]); ok {
			out.Migrate.DryRun = vv
		}
		if vv, ok := parseBool(v["diffOnly"]); ok {
			out.Migrate.DiffOnly = vv
		}
		if vv, ok := parseBool(v["concurrentIndex"]); ok {
			out.Migrate.Concurrent = vv
		}
		if vv, ok := parseDurationAny(v["timeout"]); ok {
			out.Migrate.Timeout = vv
		}
		if vv, ok := parseDurationAny(v["lockTimeout"]); ok {
			out.Migrate.LockTimeout = vv
		}
		if vv, ok := parseInt64(v["retry"]); ok {
			out.Migrate.Retry = int(vv)
		}
		if vv, ok := parseDurationAny(v["retryDelay"]); ok {
			out.Migrate.RetryDelay = vv
		}
		if vv, ok := parseDurationAny(v["jitter"]); ok {
			out.Migrate.Jitter = vv
		}
	}
	if v, ok := cfg["setting"].(Map); ok {
		out.Setting = v
	}
	out.Trash = normalizeTrashOptions(out.Trash)

	m.configs[name] = out
	return nil
}

func configuredURL(cfg Map, fallback string) (string, error) {
	envName := ""
	for _, key := range []string{"url_env", "urlEnv", "dsn_env", "dsnEnv"} {
		if value, ok := cfg[key].(string); ok && strings.TrimSpace(value) != "" {
			envName = strings.TrimSpace(value)
			break
		}
	}
	if envName == "" {
		return fallback, nil
	}
	if !validEnvironmentName(envName) {
		return "", fmt.Errorf("data: invalid DSN environment variable name %q", envName)
	}
	value, exists := os.LookupEnv(envName)
	value = strings.TrimSpace(value)
	if !exists || value == "" {
		return "", fmt.Errorf("data: required DSN environment variable %s is empty", envName)
	}
	return value, nil
}

func validEnvironmentName(name string) bool {
	for index, char := range name {
		if (char >= 'A' && char <= 'Z') || char == '_' || (index > 0 && char >= '0' && char <= '9') {
			continue
		}
		return false
	}
	return name != ""
}

func isDataReservedMapKey(key string) bool {
	_, ok := dataConfigOptionKeys[strings.ToLower(strings.TrimSpace(key))]
	return ok
}

var dataConfigOptionKeys = map[string]struct{}{
	"type": {},
	"url":  {}, "url_env": {}, "urlenv": {}, "dsn_env": {}, "dsnenv": {},
	"schema": {}, "mapping": {}, "fieldmapping": {}, "field_mapping": {},
	"readonly": {}, "read_only": {}, "trash": {}, "trashenable": {}, "trash_enable": {},
	"trashfield": {}, "trash_field": {}, "trashvalue": {}, "trash_value": {},
	"trashcascade": {}, "trash_cascade": {}, "trashcascadevalue": {}, "trash_cascade_value": {},
	"watcher": {}, "maxopen": {}, "max_open": {}, "maxidle": {}, "max_idle": {},
	"maxlifetime": {}, "max_lifetime": {}, "maxidletime": {}, "max_idle_time": {},
	"pool": {}, "migrate": {}, "setting": {},
}

func isSingleConnectionConfig(config Map) bool {
	for key := range config {
		if isDataReservedMapKey(key) {
			return true
		}
	}
	return false
}

func cloneConfigMapWithout(config Map, excluded string) Map {
	out := make(Map, len(config))
	for key, value := range config {
		if strings.EqualFold(key, excluded) {
			continue
		}
		out[key] = value
	}
	return out
}

func configKeyName(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	key = strings.ReplaceAll(key, "_", "")
	key = strings.ReplaceAll(key, "-", "")
	return key
}

func configKeys(cfg Map) map[string]struct{} {
	keys := make(map[string]struct{}, len(cfg))
	for key, value := range cfg {
		name := configKeyName(key)
		if name == "" {
			continue
		}
		keys[name] = struct{}{}
		if nested, ok := value.(Map); ok {
			for nestedKey := range nested {
				child := configKeyName(nestedKey)
				if child != "" {
					keys[name+"."+child] = struct{}{}
				}
			}
		}
	}
	return keys
}

func configKeyPresent(keys map[string]struct{}, names ...string) bool {
	for _, name := range names {
		if _, ok := keys[configKeyName(name)]; ok {
			return true
		}
	}
	return false
}

func mergeMapDefaults(file, defaults Map) Map {
	if len(file) == 0 && len(defaults) == 0 {
		return nil
	}
	out := cloneMap(file)
	for key, value := range defaults {
		current, exists := out[key]
		if !exists {
			out[key] = value
			continue
		}
		left, leftOK := current.(Map)
		right, rightOK := value.(Map)
		if leftOK && rightOK {
			out[key] = mergeMapDefaults(left, right)
		}
	}
	return out
}

func mergeConfigDefaults(file, defaults Config, keys map[string]struct{}) Config {
	if !configKeyPresent(keys, "type") {
		if file.Type == "" {
			file.Type = defaults.Type
		}
	}
	if !configKeyPresent(keys, "url", "url_env", "dsn_env") && file.Url == "" {
		file.Url = defaults.Url
	}
	if !configKeyPresent(keys, "schema") && file.Schema == "" {
		file.Schema = defaults.Schema
	}
	if !configKeyPresent(keys, "mapping", "fieldMapping") {
		file.Mapping = defaults.Mapping
	}
	if !configKeyPresent(keys, "maxOpen", "pool.maxOpen") && file.MaxOpen == 0 {
		file.MaxOpen = defaults.MaxOpen
	}
	if !configKeyPresent(keys, "maxIdle", "pool.maxIdle") && file.MaxIdle == 0 {
		file.MaxIdle = defaults.MaxIdle
	}
	if !configKeyPresent(keys, "maxLifetime", "pool.maxLifetime") && file.MaxLifetime == 0 {
		file.MaxLifetime = defaults.MaxLifetime
	}
	if !configKeyPresent(keys, "maxIdleTime", "pool.maxIdleTime") && file.MaxIdleTime == 0 {
		file.MaxIdleTime = defaults.MaxIdleTime
	}
	if !configKeyPresent(keys, "readOnly") {
		file.ReadOnly = defaults.ReadOnly
	}
	if !configKeyPresent(keys, "watcher") {
		file.Watcher = cloneMap(defaults.Watcher)
	} else {
		file.Watcher = mergeMapDefaults(file.Watcher, defaults.Watcher)
	}
	if !configKeyPresent(keys, "setting") {
		file.Setting = cloneMap(defaults.Setting)
	} else {
		file.Setting = mergeMapDefaults(file.Setting, defaults.Setting)
	}
	if !configKeyPresent(keys, "trash", "trashEnable", "trashField", "trashValue", "trashCascade") {
		file.Trash = defaults.Trash
	} else {
		if !configKeyPresent(keys, "trash.enable", "trashEnable") {
			file.Trash.Enable = defaults.Trash.Enable
		}
		if !configKeyPresent(keys, "trash.field", "trashField") && file.Trash.Field == "" {
			file.Trash.Field = defaults.Trash.Field
		}
		if !configKeyPresent(keys, "trash.value", "trashValue") && file.Trash.Value == "" {
			file.Trash.Value = defaults.Trash.Value
		}
		if !configKeyPresent(keys, "trash.cascade", "trashCascade", "trashCascadeValue") && file.Trash.Cascade == "" {
			file.Trash.Cascade = defaults.Trash.Cascade
		}
	}
	if !configKeyPresent(keys, "migrate", "migrate.startup", "migrate.start", "migrate.mode", "migrate.dryRun", "migrate.diffOnly", "migrate.concurrentIndex", "migrate.timeout", "migrate.lockTimeout", "migrate.retry", "migrate.retryDelay", "migrate.jitter") {
		file.Migrate = defaults.Migrate
	} else {
		if !configKeyPresent(keys, "migrate.startup", "migrate.start") && file.Migrate.Startup == "" {
			file.Migrate.Startup = defaults.Migrate.Startup
		}
		if !configKeyPresent(keys, "migrate.mode") && file.Migrate.Mode == "" {
			file.Migrate.Mode = defaults.Migrate.Mode
		}
		if !configKeyPresent(keys, "migrate.dryRun") {
			file.Migrate.DryRun = defaults.Migrate.DryRun
		}
		if !configKeyPresent(keys, "migrate.diffOnly") {
			file.Migrate.DiffOnly = defaults.Migrate.DiffOnly
		}
		if !configKeyPresent(keys, "migrate.concurrentIndex") {
			file.Migrate.Concurrent = defaults.Migrate.Concurrent
		}
		if !configKeyPresent(keys, "migrate.timeout") && file.Migrate.Timeout == 0 {
			file.Migrate.Timeout = defaults.Migrate.Timeout
		}
		if !configKeyPresent(keys, "migrate.lockTimeout") && file.Migrate.LockTimeout == 0 {
			file.Migrate.LockTimeout = defaults.Migrate.LockTimeout
		}
		if !configKeyPresent(keys, "migrate.retry") && file.Migrate.Retry == 0 {
			file.Migrate.Retry = defaults.Migrate.Retry
		}
		if !configKeyPresent(keys, "migrate.retryDelay") && file.Migrate.RetryDelay == 0 {
			file.Migrate.RetryDelay = defaults.Migrate.RetryDelay
		}
		if !configKeyPresent(keys, "migrate.jitter") && file.Migrate.Jitter == 0 {
			file.Migrate.Jitter = defaults.Migrate.Jitter
		}
	}
	return file
}

func parseDurationAny(v Any) (time.Duration, bool) {
	switch vv := v.(type) {
	case string:
		d, err := time.ParseDuration(strings.TrimSpace(vv))
		if err != nil || d <= 0 {
			return 0, false
		}
		return d, true
	case int:
		if vv <= 0 {
			return 0, false
		}
		return time.Duration(vv) * time.Millisecond, true
	case int64:
		if vv <= 0 {
			return 0, false
		}
		return time.Duration(vv) * time.Millisecond, true
	case float64:
		if vv <= 0 {
			return 0, false
		}
		return time.Duration(vv) * time.Millisecond, true
	default:
		return 0, false
	}
}

func parseIntAny(v Any) (int, bool) {
	switch vv := v.(type) {
	case int:
		if vv < 0 {
			return 0, false
		}
		return vv, true
	case int64:
		if vv < 0 {
			return 0, false
		}
		return int(vv), true
	case float64:
		if vv < 0 {
			return 0, false
		}
		return int(vv), true
	default:
		return 0, false
	}
}

func (m *Module) Setup(*dubi.Context) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.initialized {
		return nil
	}
	if len(m.configs) == 0 {
		return nil
	}
	for name, cfg := range m.configs {
		if name == "" {
			delete(m.configs, name)
			name = defaultName
		}
		if cfg.Type == "" {
			return fmt.Errorf("data connection %q: type is required", name)
		}
		if strings.TrimSpace(cfg.Schema) == "" {
			if schema := defaultSchemaByType(cfg.Type); schema != "" {
				cfg.Schema = schema
			}
		}
		cfg.Trash = normalizeTrashOptions(cfg.Trash)
		m.configs[name] = cfg
	}
	m.initialized = true
	return nil
}

func defaultSchemaByType(typ string) string {
	d := strings.ToLower(strings.TrimSpace(typ))
	switch d {
	case "postgresql", "postgres", "pgsql":
		return "public"
	default:
		return ""
	}
}

func (m *Module) Open(*dubi.Context) error {
	m.mutex.RLock()
	if m.connected {
		m.mutex.RUnlock()
		return nil
	}
	configs := make(map[string]Config, len(m.configs))
	for name, cfg := range m.configs {
		configs[name] = cfg
	}
	m.mutex.RUnlock()

	type openedConnection struct {
		name       string
		instance   *Instance
		connection Connection
		dispatcher *changeDispatcher
	}
	opened := make([]openedConnection, 0, len(configs))
	closeOpened := func() {
		for i := len(opened) - 1; i >= 0; i-- {
			if opened[i].dispatcher != nil {
				opened[i].dispatcher.close()
			}
			if opened[i].connection != nil {
				_ = opened[i].connection.Close()
			}
		}
	}
	names := make([]string, 0, len(configs))
	for name := range configs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		cfg := configs[name]
		inst := &Instance{Name: name, Config: cfg, Setting: cfg.Setting}
		conn, err := connectBuiltin(inst)
		if err != nil {
			closeOpened()
			return fmt.Errorf("failed to connect data %q: %w", name, err)
		}
		if err := conn.Open(); err != nil {
			_ = conn.Close()
			closeOpened()
			return fmt.Errorf("failed to open data %q: %w", name, err)
		}
		if db := conn.DB(); db != nil {
			if cfg.MaxOpen > 0 {
				db.SetMaxOpenConns(cfg.MaxOpen)
			}
			if cfg.MaxIdle > 0 {
				db.SetMaxIdleConns(cfg.MaxIdle)
			}
			if cfg.MaxLifetime > 0 {
				db.SetConnMaxLifetime(cfg.MaxLifetime)
			}
			if cfg.MaxIdleTime > 0 {
				db.SetConnMaxIdleTime(cfg.MaxIdleTime)
			}
		}
		inst.conn = conn
		opened = append(opened, openedConnection{
			name:       name,
			instance:   inst,
			connection: conn,
			dispatcher: newChangeDispatcher(name, m.parseChangeConfig(cfg), m),
		})
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.connected {
		for i := len(opened) - 1; i >= 0; i-- {
			opened[i].dispatcher.close()
			_ = opened[i].connection.Close()
		}
		return nil
	}
	for _, item := range opened {
		m.instances[item.name] = item.instance
		m.dispatchers[item.name] = item.dispatcher
	}
	m.connected = true
	return nil
}

func (m *Module) Start(*dubi.Context) error {
	m.mutex.Lock()
	if m.started {
		m.mutex.Unlock()
		return nil
	}
	m.started = true
	type startTarget struct {
		name    string
		startup string
	}
	targets := make([]startTarget, 0, len(m.instances))
	for name, inst := range m.instances {
		targets = append(targets, startTarget{
			name:    name,
			startup: normalizeMigrateStartup(inst.Config.Migrate.Startup),
		})
	}
	m.mutex.Unlock()
	sort.Slice(targets, func(i, j int) bool { return targets[i].name < targets[j].name })

	for _, target := range targets {
		db := m.Base(target.name)
		if db == nil {
			continue
		}
		var migrationErr error
		func() {
			defer db.Close()
			switch resolveMigrateStartup(target.startup) {
			case "off":
				return
			case "auto":
				db.Migrate()
				if err := db.Error(); err != nil {
					migrationErr = fmt.Errorf("data migrate(auto) failed on %s: %w", target.name, err)
					return
				}
				log.Printf("dubi data: migration completed connection=%s mode=auto", target.name)
			case "check":
				report := db.MigrateDiff()
				if err := db.Error(); err != nil {
					migrationErr = fmt.Errorf("data migrate(check) failed on %s: %w", target.name, err)
					return
				}
				if len(report.Actions) > 0 {
					migrationErr = fmt.Errorf("data migrate(check) drift detected on %s: %d actions", target.name, len(report.Actions))
					return
				}
				log.Printf("dubi data: migration check passed connection=%s mode=check", target.name)
			}
		}()
		if migrationErr != nil {
			m.mutex.Lock()
			m.started = false
			m.mutex.Unlock()
			return migrationErr
		}
	}

	if len(targets) > 0 {
		log.Printf("dubi data: module started connections=%d", len(targets))
	}
	return nil
}

func resolveMigrateStartup(startup string) string {
	mode := normalizeMigrateStartup(startup)
	if mode != "role" {
		return mode
	}
	role := strings.TrimSpace(dubi.DefaultRuntime().Identity().Role)
	if role == "" {
		role = strings.TrimSpace(os.Getenv("DUBI_ROLE"))
	}
	role = strings.ToLower(role)
	switch role {
	case "migrator", "migration", "migrate", "schema", "schema-migrator":
		return "auto"
	case "app", "api", "worker", "web":
		return "check"
	case "off", "disabled":
		return "off"
	default:
		return "off"
	}
}

func (m *Module) Stop(*dubi.Context) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if !m.started {
		return nil
	}
	m.started = false
	return nil
}

func (m *Module) Close(*dubi.Context) error {
	m.mutex.Lock()
	connections := make([]Connection, 0, len(m.instances))
	names := make([]string, 0, len(m.instances))
	for _, inst := range m.instances {
		if inst != nil && inst.conn != nil {
			connections = append(connections, inst.conn)
			names = append(names, inst.Name)
		}
	}
	dispatchers := make([]*changeDispatcher, 0, len(m.dispatchers))
	for _, dispatcher := range m.dispatchers {
		if dispatcher != nil {
			dispatchers = append(dispatchers, dispatcher)
		}
	}
	m.instances = make(map[string]*Instance, 0)
	m.dispatchers = make(map[string]*changeDispatcher, 0)
	m.connected = false
	m.initialized = false
	m.started = false
	m.mutex.Unlock()

	for i := len(dispatchers) - 1; i >= 0; i-- {
		dispatchers[i].close()
	}
	for i := len(connections) - 1; i >= 0; i-- {
		_ = connections[i].Close()
	}
	for _, name := range names {
		resetRuntimeState(name)
		resetMongoCache(name)
		resetSQLPlans(name)
	}
	return nil
}

func (m *Module) Reset() {
	_ = m.Close(nil)
	m.mutex.Lock()
	m.configs = make(Configs)
	m.fileConfigs = make(map[string]bool)
	m.fileConfigKeys = make(map[string]map[string]struct{})
	m.instances = make(map[string]*Instance)
	m.tables = make(map[string]Table)
	m.views = make(map[string]View)
	m.models = make(map[string]Model)
	m.migrations = make(map[string]Migration)
	m.watchers = make(map[string]Watcher)
	m.dispatchers = make(map[string]*changeDispatcher)
	m.mutex.Unlock()
}

func (m *Module) GetCapabilities(names ...string) (Capabilities, error) {
	base := m.Base(names...)
	defer base.Close()
	caps := base.Capabilities()
	if caps.Dialect == "" {
		return Capabilities{}, errInvalidConnection
	}
	return caps, nil
}

func (m *Module) PoolStats(names ...string) []PoolStats {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	out := make([]PoolStats, 0, len(m.instances))
	allow := map[string]struct{}{}
	if len(names) > 0 {
		for _, name := range names {
			name = strings.TrimSpace(name)
			if name != "" {
				allow[name] = struct{}{}
			}
		}
	}

	for name, inst := range m.instances {
		if len(allow) > 0 {
			if _, ok := allow[name]; !ok {
				continue
			}
		}
		ps := PoolStats{Name: name, Type: inst.Config.Type}
		if db := inst.conn.DB(); db != nil {
			stats := db.Stats()
			ps.Open = stats.OpenConnections
			ps.InUse = stats.InUse
			ps.Idle = stats.Idle
			ps.WaitCount = stats.WaitCount
			ps.WaitDuration = stats.WaitDuration.Milliseconds()
			ps.MaxOpen = stats.MaxOpenConnections
		}
		ss := m.QueryStats(name)
		ps.Queries = ss.Queries
		ps.Writes = ss.Writes
		ps.Errors = ss.Errors
		ps.CacheHit = ss.CacheHit
		ps.CacheRate = ss.CacheRate
		ps.Slow = ss.Slow
		ps.SlowAvgMs = ss.SlowAvgMs
		ps.SlowP50Ms = ss.SlowP50Ms
		ps.SlowP95Ms = ss.SlowP95Ms
		out = append(out, ps)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
