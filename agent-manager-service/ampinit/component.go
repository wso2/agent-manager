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

package ampinit

import (
	"errors"
	"os"
	"sort"
	"sync"
)

// Component writes the configuration of a service at "amp init <name> <dir>".
// ThunderID and file-based sign-in are the core's; a module that ships a
// service registers its own.
type Component struct {
	// Name is how "amp init" names it.
	Name string
	// Title names it in messages.
	Title string
	// Write reads the component's settings from the environment and writes
	// its files into dir.
	Write func(dir string) error
}

var (
	componentsMu sync.Mutex
	components   = map[string]Component{}
)

// RegisterComponent adds a component. It panics on a name taken twice.
func RegisterComponent(c Component) {
	componentsMu.Lock()
	defer componentsMu.Unlock()
	if _, ok := components[c.Name]; ok || c.Name == "thunder" || c.Name == "fileauth" {
		panic("ampinit: component " + c.Name + " registered twice")
	}
	components[c.Name] = c
}

// Components returns the names of every component, the core's first.
func Components() []string {
	componentsMu.Lock()
	defer componentsMu.Unlock()
	names := make([]string, 0, len(components))
	for n := range components {
		names = append(names, n)
	}
	sort.Strings(names)
	return append([]string{"thunder", "fileauth"}, names...)
}

func component(name string) (Component, bool) {
	componentsMu.Lock()
	defer componentsMu.Unlock()
	c, ok := components[name]
	return c, ok
}

// Env reads a component's settings from the environment, collecting every
// problem so they are reported together.
type Env struct{ r envReader }

// Str is the variable, or def when it is unset or blank.
func (e *Env) Str(key, def string) string { return e.r.str(key, def) }

// Required is the variable, recording an error when it is unset or blank.
func (e *Env) Required(key string) string { return e.r.required(key) }

// Port is the variable as a TCP port, or def.
func (e *Env) Port(key string, def int) int { return e.r.port(key, def) }

// Fail records a problem.
func (e *Env) Fail(err error) { e.r.errs = append(e.r.errs, err) }

// Err is every problem recorded, or nil.
func (e *Env) Err() error { return errors.Join(e.r.errs...) }

// Owner is the user and group a written file is given.
type Owner = owner

// ParseOwner reads "uid:gid" from the variable; nil when it is unset.
func ParseOwner(key string) (*Owner, error) { return parseOwner(key) }

// WriteFile writes a file with the mode and owner.
func WriteFile(path string, data []byte, mode os.FileMode, o *Owner) error {
	return writeFile(path, data, mode, o)
}

// WriteOnce writes the file gen returns unless it exists, and returns the
// file's content either way.
func WriteOnce(path string, mode os.FileMode, o *Owner, gen func() ([]byte, error)) ([]byte, error) {
	return writeOnce(path, mode, o, gen)
}

// ChownDir creates the directory and gives it to the owner.
func ChownDir(dir string, o *Owner) error { return chownDir(dir, o) }

// Quote is s as a double-quoted literal, valid in YAML, JavaScript and TOML.
func Quote(s string) (string, error) { return quote(s) }

// Settle gives an existing file the mode and owner.
func Settle(path string, mode os.FileMode, o *Owner) error { return settle(path, mode, o) }
