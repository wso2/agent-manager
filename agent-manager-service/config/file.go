// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// The configuration file. Every key in it is a setting's environment variable
// name written as nested YAML: `db: {driver: postgres}` is DB_DRIVER,
// `console: {auth_client_id: x}` is CONSOLE_AUTH_CLIENT_ID. A variable set in
// the environment, even to "", always wins over the file, so anything the file
// says can be overridden per deployment, and a setup that uses only the
// environment works unchanged. File values never enter the process
// environment: settings are read with Value.
const (
	// FileEnv names the configuration file when --config does not. Unset,
	// DefaultFile in the working directory is read if it exists.
	FileEnv = "AMP_CONFIG"
	// DefaultFile is the configuration file read when nothing names one.
	DefaultFile = "amp.yaml"
	// ModulesEnv overrides the file's modules list: comma-separated module
	// names, each optionally followed by "=off" to keep it installed but
	// disabled ("gateway,widgets=off"), or "none" for an empty list.
	ModulesEnv = "AMP_MODULES"
)

// Prefixes of setting families whose keys are open-ended: the console's
// config.js placeholders and modules' own settings.
const (
	ConsolePrefix = "CONSOLE_"
	ModulePrefix  = "MODULE_"
)

var (
	fileMu sync.RWMutex
	// fileVars are the configuration file's settings, by variable name.
	fileVars map[string]string
	// fileModules is the modules list the configuration file declared.
	fileModules Modules

	// readKeys records the settings looked up while tracking is on (during
	// Load), and registered the settings other packages read; together they
	// tell a misspelled file key from a real one.
	readKeys   = map[string]bool{}
	registered = map[string]bool{}
	tracking   bool
	// fileErrs are the _FILE settings Load could not read.
	fileErrs []error
)

// RegisterKeys declares settings a package reads with Value outside Load
// (for example `amp init`'s), so the configuration file may set them.
func RegisterKeys(keys ...string) {
	fileMu.Lock()
	defer fileMu.Unlock()
	for _, k := range keys {
		registered[k] = true
	}
}

// FileSuffix names a setting's file: KEY_FILE holds a path whose contents
// are KEY's value.
const FileSuffix = "_FILE"

// Lookup returns a setting, first found of: the environment variable key
// (even ""), the file the environment's key_FILE names, the configuration
// file's key, the file the configuration file's key_file names.
func Lookup(key string) (string, bool) {
	fileMu.RLock()
	track := tracking
	v, inFile := fileVars[key]
	vFile, inFileFile := fileVars[key+FileSuffix]
	fileMu.RUnlock()
	if track {
		fileMu.Lock()
		readKeys[key] = true
		fileMu.Unlock()
	}
	if env, ok := os.LookupEnv(key); ok {
		return env, true
	}
	if path, ok := os.LookupEnv(key + FileSuffix); ok && path != "" {
		return readSettingFile(key+FileSuffix, path)
	}
	if inFile {
		return v, true
	}
	if inFileFile && vFile != "" {
		return readSettingFile(key+FileSuffix, vFile)
	}
	return "", false
}

// readSettingFile reads a _FILE setting's file. A failure is reported by
// Load when it happens there, and logged otherwise.
func readSettingFile(name, path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err == nil {
		return trimNewline(string(data)), true
	}
	err = fmt.Errorf("%s: %w", name, err)
	fileMu.Lock()
	defer fileMu.Unlock()
	if tracking {
		fileErrs = append(fileErrs, err)
	} else {
		slog.Error("reading a setting's file failed", "error", err)
	}
	return "", false
}

// Value is Lookup's value, "" when unset.
func Value(key string) string {
	v, _ := Lookup(key)
	return v
}

// ModuleSpec is one entry of the declarative modules list.
type ModuleSpec struct {
	Name string
	// Enabled is false for a module that should be installed but disabled.
	Enabled bool
}

// Modules is the declarative modules list, reconciled at boot. Declared is
// false when neither the file nor ModulesEnv has a list; then nothing is
// reconciled. An empty declared list (`modules: []`, AMP_MODULES=none)
// disables every module that is not built in.
type Modules struct {
	Declared bool
	List     []ModuleSpec
}

// GetModules returns the declarative modules list, ModulesEnv first.
func GetModules() (Modules, error) {
	if v := strings.TrimSpace(os.Getenv(ModulesEnv)); v != "" {
		return parseModulesEnv(v)
	}
	fileMu.RLock()
	defer fileMu.RUnlock()
	return fileModules, nil
}

var moduleNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

func parseModulesEnv(v string) (Modules, error) {
	if strings.EqualFold(v, "none") {
		return Modules{Declared: true}, nil
	}
	var m Modules
	for _, part := range strings.Split(v, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, state, hasState := strings.Cut(part, "=")
		spec := ModuleSpec{Name: strings.TrimSpace(name), Enabled: true}
		if hasState {
			on, err := parseOnOff(strings.TrimSpace(state))
			if err != nil {
				return Modules{}, fmt.Errorf("%s: module %q: %w", ModulesEnv, spec.Name, err)
			}
			spec.Enabled = on
		}
		m.List = append(m.List, spec)
	}
	// Only separators ("," or " , ") declare nothing; "none" is the empty list.
	m.Declared = len(m.List) > 0
	return m, checkModules(m.List, ModulesEnv)
}

func parseOnOff(s string) (bool, error) {
	switch strings.ToLower(s) {
	case "on", "true", "enabled":
		return true, nil
	case "off", "false", "disabled":
		return false, nil
	}
	return false, fmt.Errorf("state %q is not on or off", s)
}

func checkModules(list []ModuleSpec, source string) error {
	seen := map[string]bool{}
	for _, s := range list {
		if !moduleNamePattern.MatchString(s.Name) {
			return fmt.Errorf("%s: %q is not a module name", source, s.Name)
		}
		if seen[s.Name] {
			return fmt.Errorf("%s: module %q is listed twice", source, s.Name)
		}
		seen[s.Name] = true
	}
	return nil
}

// ModuleConfigKey is the setting that holds key of a module's configuration:
// MODULE_<MODULE>_<KEY>, upper-cased, "-" and "." written as "_".
func ModuleConfigKey(module, key string) string {
	return envName("module_" + module + "_" + key)
}

// configFilePath returns the file to read and whether it was named (a named
// file that is missing is an error; a missing default is not).
func configFilePath(flag string) (string, bool) {
	if p := strings.TrimSpace(flag); p != "" {
		return p, true
	}
	if p := strings.TrimSpace(os.Getenv(FileEnv)); p != "" {
		return p, true
	}
	return DefaultFile, false
}

// Load reads the configuration file (path, else $AMP_CONFIG, else ./amp.yaml
// if present) and then every setting again, with the environment over the
// file over the defaults. The command line calls it once, after parsing its
// flags and before anything else reads the configuration. File keys that no
// setting reads are logged.
func Load(path string) error {
	vars, mods, err := readConfigFile(path)
	if err != nil {
		return err
	}
	fileMu.Lock()
	fileVars, fileModules = vars, mods
	readKeys = map[string]bool{}
	tracking = true
	fileErrs = nil
	fileMu.Unlock()

	// Not strict yet: an unreadable _FILE is reported before the settings it
	// left empty.
	loadSettings(false)

	fileMu.Lock()
	tracking = false
	unknown := unknownKeys(vars)
	readErr := errors.Join(fileErrs...)
	fileMu.Unlock()
	if readErr != nil {
		return readErr
	}
	GetConfig()
	if len(unknown) > 0 {
		slog.Warn("configuration file sets keys no setting reads; check their spelling", "keys", unknown)
	}
	return nil
}

func readConfigFile(flag string) (map[string]string, Modules, error) {
	path, named := configFilePath(flag)
	data, err := os.ReadFile(path)
	if err != nil {
		if !named && errors.Is(err, os.ErrNotExist) {
			return nil, Modules{}, nil
		}
		return nil, Modules{}, fmt.Errorf("reading %s: %w", path, err)
	}
	vars, mods, err := parseConfigFile(data)
	if err != nil {
		return nil, Modules{}, fmt.Errorf("%s: %w", path, err)
	}
	var errs []error
	for k, v := range vars {
		if vars[k], err = resolveRef(k, v, os.LookupEnv, os.ReadFile); err != nil {
			errs = append(errs, err)
		}
	}
	if err := errors.Join(errs...); err != nil {
		return nil, Modules{}, fmt.Errorf("%s: %w", path, err)
	}
	return vars, mods, nil
}

// unknownKeys lists the file's keys that no setting read during Load and no
// package registered. Callers hold fileMu.
func unknownKeys(vars map[string]string) []string {
	var out []string
	for k := range vars {
		base := strings.TrimSuffix(k, FileSuffix)
		if readKeys[k] || registered[k] || readKeys[base] || registered[base] ||
			strings.HasPrefix(k, ConsolePrefix) || strings.HasPrefix(k, ModulePrefix) {
			continue
		}
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// parseConfigFile flattens a configuration file into settings and its modules
// list.
func parseConfigFile(data []byte) (map[string]string, Modules, error) {
	var root map[string]any
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, Modules{}, err
	}
	vars := map[string]string{}
	var mods Modules
	for key, value := range root {
		if strings.EqualFold(key, "modules") {
			var err error
			if mods, err = parseModulesNode(value, vars); err != nil {
				return nil, Modules{}, err
			}
			continue
		}
		if err := flatten(key, value, vars); err != nil {
			return nil, Modules{}, err
		}
	}
	return vars, mods, nil
}

// parseModulesNode reads the modules list. An entry is a module name, or a
// mapping with name, enabled (default true) and config (the module's own
// settings, flattened under MODULE_<NAME>_). A key with no value (every entry
// commented out) declares nothing; `modules: []` declares an empty list.
func parseModulesNode(value any, vars map[string]string) (Modules, error) {
	if value == nil {
		return Modules{}, nil
	}
	list, ok := value.([]any)
	if !ok {
		return Modules{}, errors.New("modules: want a list of module names or {name, enabled, config} entries")
	}
	m := Modules{Declared: true}
	blank := 0
	for i, item := range list {
		switch e := item.(type) {
		case nil:
			blank++
		case string:
			if strings.TrimSpace(e) == "" {
				blank++
				continue
			}
			m.List = append(m.List, ModuleSpec{Name: e, Enabled: true})
		case map[string]any:
			spec, err := parseModuleEntry(i, e, vars)
			if err != nil {
				return Modules{}, err
			}
			m.List = append(m.List, spec)
		default:
			return Modules{}, fmt.Errorf("modules[%d]: want a module name or a mapping", i)
		}
	}
	if len(list) > 0 && blank == len(list) {
		return Modules{}, nil
	}
	return m, checkModules(m.List, "modules")
}

func parseModuleEntry(i int, e map[string]any, vars map[string]string) (ModuleSpec, error) {
	spec := ModuleSpec{Enabled: true}
	for k, v := range e {
		switch k {
		case "name":
			s, ok := v.(string)
			if !ok {
				return spec, fmt.Errorf("modules[%d].name: want a string", i)
			}
			spec.Name = s
		case "enabled":
			b, ok := v.(bool)
			if !ok {
				return spec, fmt.Errorf("modules[%d].enabled: want true or false", i)
			}
			spec.Enabled = b
		case "config":
		default:
			return spec, fmt.Errorf("modules[%d]: unknown key %q (want name, enabled, config)", i, k)
		}
	}
	if c, ok := e["config"]; ok && c != nil {
		if _, isMap := c.(map[string]any); !isMap {
			return spec, fmt.Errorf("modules[%d].config: want a mapping", i)
		}
		if err := flatten("module_"+spec.Name, c, vars); err != nil {
			return spec, err
		}
	}
	return spec, nil
}

func flatten(prefix string, value any, vars map[string]string) error {
	switch v := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if err := flatten(prefix+"_"+k, v[k], vars); err != nil {
				return err
			}
		}
		return nil
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			s, err := scalar(prefix, item)
			if err != nil {
				return err
			}
			parts = append(parts, s)
		}
		return set(vars, prefix, strings.Join(parts, ","))
	default:
		s, err := scalar(prefix, v)
		if err != nil {
			return err
		}
		return set(vars, prefix, s)
	}
}

func set(vars map[string]string, key, value string) error {
	name := envName(key)
	if prev, dup := vars[name]; dup && prev != value {
		return fmt.Errorf("%s is set twice with different values", name)
	}
	vars[name] = value
	return nil
}

func scalar(key string, v any) (string, error) {
	switch s := v.(type) {
	case nil:
		return "", nil
	case string:
		return s, nil
	case bool:
		return strconv.FormatBool(s), nil
	case int:
		return strconv.Itoa(s), nil
	case int64:
		return strconv.FormatInt(s, 10), nil
	case uint64:
		return strconv.FormatUint(s, 10), nil
	case float64:
		return strconv.FormatFloat(s, 'f', -1, 64), nil
	}
	return "", fmt.Errorf("%s: want a value, a list of values or a mapping", envName(key))
}

func envName(key string) string {
	return strings.ToUpper(strings.NewReplacer("-", "_", ".", "_").Replace(key))
}
