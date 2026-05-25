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

//go:build ignore

// Run via: go run ./scripts/gen_instrumentation_baseline.go
//
// Reads .github/release-config.json (relative to repo root) and writes
// instrumentation/baseline.json. Keeps the bundled instrumentation
// baseline in sync with the CI release config without runtime parsing.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const defaultImageRepo = "ghcr.io/wso2/amp-python-instrumentation-provider"

type releaseEntry struct {
	InstrumentationVersion string   `json:"instrumentation_version"`
	TraceloopVersion       string   `json:"traceloop_version"`
	PythonVersions         []string `json:"python_versions"`
}

type releaseConfig struct {
	PythonProvider []releaseEntry `json:"python-instrumentation-provider"`
}

type baselineEntry struct {
	Version         string   `json:"version"`
	TraceloopSDK    string   `json:"traceloopSdk"`
	PythonVersions  []string `json:"pythonVersions"`
	ImageRepository string   `json:"imageRepository"`
}

func main() {
	wd, err := os.Getwd()
	if err != nil {
		fail(err)
	}
	repoRoot := filepath.Clean(filepath.Join(wd, ".."))
	srcPath := filepath.Join(repoRoot, ".github", "release-config.json")
	dstPath := filepath.Join(wd, "instrumentation", "baseline.json")

	raw, err := os.ReadFile(srcPath)
	if err != nil {
		fail(fmt.Errorf("read %s: %w", srcPath, err))
	}
	var cfg releaseConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		fail(fmt.Errorf("parse %s: %w", srcPath, err))
	}
	out := make([]baselineEntry, 0, len(cfg.PythonProvider))
	for _, e := range cfg.PythonProvider {
		out = append(out, baselineEntry{
			Version:         e.InstrumentationVersion,
			TraceloopSDK:    e.TraceloopVersion,
			PythonVersions:  e.PythonVersions,
			ImageRepository: defaultImageRepo,
		})
	}
	body, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fail(err)
	}
	body = append(body, '\n')
	if err := os.WriteFile(dstPath, body, 0o644); err != nil {
		fail(err)
	}
	fmt.Printf("wrote %d entries to %s\n", len(out), dstPath)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
