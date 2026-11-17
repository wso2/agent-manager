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
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// The configuration file. Every key in it is an environment variable name
// written as nested YAML: `db: {driver: postgres}` is DB_DRIVER, `console:
// {auth_client_id: x}` is CONSOLE_AUTH_CLIENT_ID. A variable set in the
// environment always wins over the file, so anything the file says can be
// overridden per deployment, and a setup that uses only the environment
// works unchanged.
const (
	// FileEnv names the configuration file. Unset, DefaultFile in the working
	// directory is read if it exists.
	FileEnv = "AMP_CONFIG"
	// DefaultFile is the configuration file read when FileEnv is unset.
	DefaultFile = "amp.yaml"
	// ModulesEnv overrides the file's modules list: comma-separated module
	// names, each optionally followed by "=off" to keep it installed but
	// disabled ("gateway,widgets=off").
	ModulesEnv = "AMP_MODULES"
)

// ModuleSpec is one entry of the declarative modules list.
type ModuleSpec struct {
	Name string
	// Enabled is false for a module that should be installed but disabled.
	Enabled bool
}

// Modules is the declarative modules list, reconciled at boot: each listed
// module is installed (with what it requires) and enabled or disabled to
// match; installed modules not listed are disabled. Declared is false when
// neither the file nor ModulesEnv has a list, and nothing is reconciled.
type Modules struct {
	Declared bool
	List     []ModuleSpec
}

// fileModules is the modules list the configuration file declared.
var fileModules Modules

// GetModules returns the declarative modules list, ModulesEnv first.
func GetModules() (Modules, error) {
	if v := strings.TrimSpace(os.Getenv(ModulesEnv)); v != "" {
		return parseModulesEnv(v)
	}
	return fileModules, nil
}

var moduleNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

func parseModulesEnv(v string) (Modules, error) {
	m := Modules{Declared: true}
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

// ModuleConfigEnv is the environment variable that holds key of a module's
// configuration: MODULE_<MODULE>_<KEY>, upper-cased, "-" and "." written as "_".
func ModuleConfigEnv(module, key string) string {
	return envName("module_" + module + "_" + key)
}

// configFilePath returns the file to read and whether it was named
// explicitly (a missing named file is an error; a missing default is not).
func configFilePath() (string, bool) {
	if p := strings.TrimSpace(os.Getenv(FileEnv)); p != "" {
		return p, true
	}
	return DefaultFile, false
}

// applyConfigFile reads the configuration file, if any, into the
// environment: each key is set only where the environment has no value.
func applyConfigFile() error {
	fileModules = Modules{}
	path, explicit := configFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if !explicit && errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("reading %s: %w", path, err)
	}
	vars, mods, err := parseConfigFile(data)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	for k, v := range vars {
		if os.Getenv(k) == "" {
			if err := os.Setenv(k, v); err != nil {
				return fmt.Errorf("%s: setting %s: %w", path, k, err)
			}
		}
	}
	fileModules = mods
	return nil
}

// parseConfigFile flattens a configuration file into environment variables
// and its modules list.
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
// settings, flattened under MODULE_<NAME>_).
func parseModulesNode(value any, vars map[string]string) (Modules, error) {
	m := Modules{Declared: true}
	if value == nil {
		return m, nil
	}
	list, ok := value.([]any)
	if !ok {
		return Modules{}, errors.New("modules: want a list of module names or {name, enabled, config} entries")
	}
	for i, item := range list {
		switch e := item.(type) {
		case string:
			m.List = append(m.List, ModuleSpec{Name: e, Enabled: true})
		case map[string]any:
			spec := ModuleSpec{Enabled: true}
			for k, v := range e {
				switch k {
				case "name":
					s, ok := v.(string)
					if !ok {
						return Modules{}, fmt.Errorf("modules[%d].name: want a string", i)
					}
					spec.Name = s
				case "enabled":
					b, ok := v.(bool)
					if !ok {
						return Modules{}, fmt.Errorf("modules[%d].enabled: want true or false", i)
					}
					spec.Enabled = b
				case "config":
				default:
					return Modules{}, fmt.Errorf("modules[%d]: unknown key %q (want name, enabled, config)", i, k)
				}
			}
			if c, ok := e["config"]; ok && c != nil {
				if _, isMap := c.(map[string]any); !isMap {
					return Modules{}, fmt.Errorf("modules[%d].config: want a mapping", i)
				}
				if err := flatten("module_"+spec.Name, c, vars); err != nil {
					return Modules{}, err
				}
			}
			m.List = append(m.List, spec)
		default:
			return Modules{}, fmt.Errorf("modules[%d]: want a module name or a mapping", i)
		}
	}
	return m, checkModules(m.List, "modules")
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

// Reload reads the configuration file named by FileEnv and the environment
// again. The command line calls it after --config sets FileEnv, before
// anything else reads the configuration.
func Reload() {
	loadEnvs()
}
