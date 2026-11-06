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
	"testing"
)

func TestLoad_BaselineOnly(t *testing.T) {
	c, err := Load("", "0.3.0")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Default() != "0.3.0" {
		t.Errorf("Default = %q, want 0.3.0", c.Default())
	}
	// The bundled baseline currently ships five versions. All() ordering is
	// unspecified (it comes from a map), so assert membership by version key
	// rather than by slice position.
	all := c.All()
	if len(all) != 5 {
		t.Fatalf("len(All) = %d, want 5", len(all))
	}
	for _, want := range []string{"0.3.0", "0.4.0", "0.4.1", "0.4.2", "0.5.0"} {
		got, ok := c.Get(want)
		if !ok {
			t.Errorf("Get(%s) missing from bundled baseline", want)
			continue
		}
		if got.Source != SourceBundled {
			t.Errorf("Get(%s).Source = %q, want bundled", want, got.Source)
		}
	}
	if c.Has("99.0.0") {
		t.Error("Has(99.0.0) = true, want false")
	}
	got, ok := c.Get("0.3.0")
	if !ok || got.ImageRepository != "ghcr.io/wso2/amp-python-instrumentation-provider" {
		t.Errorf("Get(0.3.0) = %+v, ok=%v", got, ok)
	}
}

func TestLoad_DefaultNotInSet(t *testing.T) {
	if _, err := Load("", "9.9.9"); err == nil {
		t.Fatal("Load with bad default returned nil error")
	}
}

func TestLoad_ExtensionAdds(t *testing.T) {
	c, err := Load("testdata/extension_valid.yaml", "0.3.0")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !c.Has("0.6.0") {
		t.Fatal("expected 0.6.0 in effective set")
	}
	got, _ := c.Get("0.6.0")
	if got.Source != SourceExtension {
		t.Errorf("Get(0.6.0).Source = %q, want extension", got.Source)
	}
	if got.TraceloopSDK != "0.65.0" {
		t.Errorf("Get(0.6.0).TraceloopSDK = %q, want 0.65.0", got.TraceloopSDK)
	}
	bundled, _ := c.Get("0.3.0")
	if bundled.Source != SourceBundled {
		t.Errorf("Get(0.3.0).Source = %q, want bundled", bundled.Source)
	}
}

func TestLoad_ExtensionOverridesBundled(t *testing.T) {
	c, err := Load("testdata/extension_override.yaml", "0.3.0")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, _ := c.Get("0.3.0")
	if got.Source != SourceExtension {
		t.Errorf("expected extension override; Source = %q", got.Source)
	}
	if got.ImageRepository != "internal.registry.example/amp-python-instrumentation-provider" {
		t.Errorf("ImageRepository = %q, want override", got.ImageRepository)
	}
}

func TestLoad_ExtensionFileAbsent(t *testing.T) {
	c, err := Load("testdata/does_not_exist.yaml", "0.3.0")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.All()) != 5 {
		t.Errorf("len(All) = %d, want 5 (baseline only)", len(c.All()))
	}
}

func TestLoad_ExtensionMalformedYAML(t *testing.T) {
	if _, err := Load("testdata/extension_malformed.yaml", "0.3.0"); err == nil {
		t.Fatal("expected error for malformed YAML")
	}
}

func TestLoad_ExtensionMissingRequiredFields(t *testing.T) {
	if _, err := Load("testdata/extension_missing_fields.yaml", "0.3.0"); err == nil {
		t.Fatal("expected error for missing imageRepository")
	}
}

func TestAllAndGet_ReturnDefensiveCopies(t *testing.T) {
	c, err := Load("", "0.3.0")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// All(): mutating the returned slice or the nested PythonVersions
	// must not affect a subsequent read.
	first := c.All()
	if len(first) == 0 || len(first[0].PythonVersions) == 0 {
		t.Fatalf("expected non-empty baseline; got %+v", first)
	}
	first[0].Version = "tampered"
	first[0].PythonVersions[0] = "tampered"

	second := c.All()
	if second[0].Version == "tampered" {
		t.Error("All() mutation leaked into catalog (Version field)")
	}
	if second[0].PythonVersions[0] == "tampered" {
		t.Error("All() mutation leaked into catalog (PythonVersions slice)")
	}

	// Get(): same contract.
	got, ok := c.Get("0.3.0")
	if !ok {
		t.Fatal("Get(0.3.0) missing")
	}
	got.PythonVersions[0] = "tampered"
	again, _ := c.Get("0.3.0")
	if again.PythonVersions[0] == "tampered" {
		t.Error("Get() mutation leaked into catalog")
	}
}

func TestSetCatalog_PanicsOnNil(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("SetCatalog(nil) should panic")
		}
	}()
	SetCatalog(nil)
}

func TestImageRepositoryFor_Catalog(t *testing.T) {
	const fallback = "fallback.test/amp-python-instrumentation-provider"
	c := NewForTest([]Version{
		{Version: "0.4.1", PythonVersions: []string{"3.11"}, ImageRepository: "mirror.test/provider"},
		{Version: "0.5.0", PythonVersions: []string{"3.11"}},
	}, "0.4.1")

	if got := c.ImageRepositoryFor("0.4.1", fallback); got != "mirror.test/provider" {
		t.Errorf("ImageRepositoryFor(0.4.1) = %q, want the catalog entry's repository", got)
	}
	// Entry present but carrying no repository, and an entirely unknown version,
	// both degrade to the caller's default rather than an empty image prefix.
	if got := c.ImageRepositoryFor("0.5.0", fallback); got != fallback {
		t.Errorf("ImageRepositoryFor(0.5.0) = %q, want fallback for an empty repository", got)
	}
	if got := c.ImageRepositoryFor("9.9.9", fallback); got != fallback {
		t.Errorf("ImageRepositoryFor(9.9.9) = %q, want fallback for an unknown version", got)
	}
}

func TestImageRepositoryFor_ProcessWide(t *testing.T) {
	const fallback = "fallback.test/amp-python-instrumentation-provider"

	// Swap the process-wide catalog out and back so this test neither depends on
	// nor leaks into the package-level state other tests share.
	pkgCatalogMu.Lock()
	saved := pkgCatalog
	pkgCatalog = nil
	pkgCatalogMu.Unlock()
	t.Cleanup(func() {
		pkgCatalogMu.Lock()
		pkgCatalog = saved
		pkgCatalogMu.Unlock()
	})

	// Unlike GetCatalog, this must not panic when the catalog was never installed.
	if got := ImageRepositoryFor("0.4.1", fallback); got != fallback {
		t.Errorf("ImageRepositoryFor with no catalog = %q, want fallback", got)
	}

	SetCatalog(NewForTest([]Version{
		{Version: "0.4.1", PythonVersions: []string{"3.11"}, ImageRepository: "mirror.test/provider"},
	}, "0.4.1"))
	if got := ImageRepositoryFor("0.4.1", fallback); got != "mirror.test/provider" {
		t.Errorf("ImageRepositoryFor = %q, want the installed catalog's repository", got)
	}
}
