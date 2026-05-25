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

package instrumentation

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

const (
	SourceBundled   = "bundled"
	SourceExtension = "extension"
)

// Version is one entry in the instrumentation catalog.
type Version struct {
	Version         string   `json:"version"          yaml:"version"`
	TraceloopSDK    string   `json:"traceloopSdk"     yaml:"traceloopSdk"`
	PythonVersions  []string `json:"pythonVersions"   yaml:"pythonVersions"`
	ImageRepository string   `json:"imageRepository"  yaml:"imageRepository"`
	Source          string   `json:"source"           yaml:"-"`
}

// Catalog is the effective instrumentation version set, assembled from
// the embedded baseline plus an optional operator-supplied extension.
type Catalog struct {
	versions       []Version
	defaultVersion string
	byVersion      map[string]Version
}

// extensionFile is the on-disk YAML shape.
type extensionFile struct {
	AdditionalInstrumentationVersions []Version `yaml:"additionalInstrumentationVersions"`
}

// Load assembles the catalog from the embedded baseline plus the optional
// extension file at extensionPath. Extension entries are validated and
// merged in; when an extension entry shares a version with a bundled
// entry the extension wins (lets an operator redirect imageRepository
// for an air-gapped mirror). The defaultVersion must appear in the
// effective set or Load returns an error. extensionPath == "" or a
// missing file is not an error; the catalog is baseline-only in that
// case.
func Load(extensionPath, defaultVersion string) (*Catalog, error) {
	baseline, err := decodeBaseline()
	if err != nil {
		return nil, err
	}

	by := make(map[string]Version, len(baseline))
	for _, v := range baseline {
		by[v.Version] = v
	}

	ext, err := readExtension(extensionPath)
	if err != nil {
		return nil, err
	}
	for _, v := range ext {
		if err := validateExtensionEntry(v); err != nil {
			return nil, fmt.Errorf("extension entry %q: %w", v.Version, err)
		}
		v.Source = SourceExtension
		by[v.Version] = v
	}

	versions := make([]Version, 0, len(by))
	for _, v := range by {
		versions = append(versions, v)
	}

	if _, ok := by[defaultVersion]; !ok {
		return nil, fmt.Errorf("default instrumentation version %q not in effective set", defaultVersion)
	}

	return &Catalog{
		versions:       versions,
		defaultVersion: defaultVersion,
		byVersion:      by,
	}, nil
}

func readExtension(path string) ([]Version, error) {
	if path == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read extension %s: %w", path, err)
	}
	var f extensionFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("parse extension %s: %w", path, err)
	}
	return f.AdditionalInstrumentationVersions, nil
}

func validateExtensionEntry(v Version) error {
	if v.Version == "" {
		return errors.New("missing version")
	}
	if v.ImageRepository == "" {
		return errors.New("missing imageRepository")
	}
	if len(v.PythonVersions) == 0 {
		return errors.New("missing pythonVersions")
	}
	return nil
}

// All returns every version in the effective catalog. Ordering is
// unspecified; callers that need deterministic ordering must sort.
// The returned slice and each entry's PythonVersions are copies so a
// caller cannot mutate catalog internals.
func (c *Catalog) All() []Version {
	out := make([]Version, len(c.versions))
	for i := range c.versions {
		out[i] = cloneVersion(c.versions[i])
	}
	return out
}

// Default returns the platform default instrumentation version.
func (c *Catalog) Default() string { return c.defaultVersion }

// Has reports whether the version is present in the effective catalog.
func (c *Catalog) Has(v string) bool {
	_, ok := c.byVersion[v]
	return ok
}

// Get returns a copy of the catalog entry for the given version, so a
// caller mutating the returned PythonVersions slice cannot corrupt the
// catalog state shared across requests.
func (c *Catalog) Get(v string) (Version, bool) {
	got, ok := c.byVersion[v]
	if !ok {
		return Version{}, false
	}
	return cloneVersion(got), true
}

// cloneVersion returns a Version whose slice fields are independent
// copies of the input's.
func cloneVersion(v Version) Version {
	cp := v
	if v.PythonVersions != nil {
		cp.PythonVersions = append([]string(nil), v.PythonVersions...)
	}
	return cp
}

// pkgCatalog is the process-wide default catalog, installed once at
// startup by the app boot path. It mirrors the config.GetConfig() pattern
// so legacy code paths can reach the catalog without taking it through
// constructor DI. New code should prefer DI through Wire; this accessor
// exists for the legacy boundary in services/agent_manager.go.
//
// In production SetCatalog runs once at boot before any reader exists,
// so a lock isn't strictly required. The mutex is here so that tests
// (which install their own NewForTest catalogs and may run in parallel
// against tests that build app params via Wire) don't trip the race
// detector. Read/write contention in production is negligible.
var (
	pkgCatalogMu sync.RWMutex
	pkgCatalog   *Catalog
)

// SetCatalog installs the process-wide catalog. Call once at startup
// after Load succeeds. Subsequent calls overwrite the prior catalog.
// Panics on nil so a boot-time wiring bug fails at the source instead
// of deferring the failure to the next GetCatalog call from a request
// handler.
func SetCatalog(c *Catalog) {
	if c == nil {
		panic("instrumentation.SetCatalog called with nil catalog")
	}
	pkgCatalogMu.Lock()
	pkgCatalog = c
	pkgCatalogMu.Unlock()
}

// GetCatalog returns the process-wide catalog. Panics if SetCatalog has
// never been called — this signals a boot-order bug, not a runtime
// condition we should handle gracefully.
func GetCatalog() *Catalog {
	pkgCatalogMu.RLock()
	c := pkgCatalog
	pkgCatalogMu.RUnlock()
	if c == nil {
		panic("instrumentation.GetCatalog called before SetCatalog")
	}
	return c
}

// NewForTest builds a catalog directly from versions. Use only in tests
// that need to populate the package-level catalog without going through
// Load (e.g. for cross-package validators that depend on GetCatalog).
func NewForTest(versions []Version, defaultVersion string) *Catalog {
	by := make(map[string]Version, len(versions))
	for _, v := range versions {
		by[v.Version] = v
	}
	return &Catalog{
		versions:       versions,
		defaultVersion: defaultVersion,
		byVersion:      by,
	}
}
