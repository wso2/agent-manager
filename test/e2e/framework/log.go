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

package framework

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	lineWidth = 70
	logsDir   = "../_logs"
)

// testLoggers stores per-test loggers, keyed by test name.
var testLoggers sync.Map

// Log writes a formatted message to the per-test log file.
// This should be used instead of t.Logf everywhere to avoid interleaved output.
func Log(t *testing.T, format string, args ...any) {
	t.Helper()
	logger := getOrCreateFileLogger(t)
	logger.write(format, args...)
}

// StepLogger provides structured, readable logging for integration test steps.
// All output goes to a per-test log file, not to stdout.
type StepLogger struct {
	t     *testing.T
	tag   string
	step  int
	start time.Time
	fl    *fileLogger
}

// NewStepLogger creates a new step logger. It registers a per-test file logger
// that all framework.Log calls for this test will also write to.
func NewStepLogger(t *testing.T, tag string) *StepLogger {
	fl := getOrCreateFileLogger(t)
	return &StepLogger{t: t, tag: tag, start: time.Now(), fl: fl}
}

func (s *StepLogger) log(format string, args ...any) {
	s.t.Helper()
	msg := fmt.Sprintf(format, args...)
	s.fl.write("[%s] %s", s.tag, msg)
}

// TestHeader prints the test name as a prominent header.
func (s *StepLogger) TestHeader(name string) {
	s.t.Helper()
	s.log("%s", strings.Repeat("=", lineWidth))
	s.log("  TEST: %s", name)
	s.log("%s", strings.Repeat("=", lineWidth))
}

// Begin starts a new numbered step with a header.
func (s *StepLogger) Begin(title string) {
	s.t.Helper()
	s.step++
	s.log("%s", strings.Repeat("-", lineWidth))
	s.log("  STEP %d: %s", s.step, title)
	s.log("%s", strings.Repeat("-", lineWidth))
}

// Info logs a key-value detail within the current step.
func (s *StepLogger) Info(key, value string) {
	s.t.Helper()
	s.log("  %-12s %s", key+":", value)
}

// Done marks the current step as complete with elapsed time.
func (s *StepLogger) Done(msg string, since time.Time) {
	s.t.Helper()
	elapsed := time.Since(since).Round(time.Millisecond)
	s.log("  [PASS] %s (%s)", msg, elapsed)
}

// Summary prints a final summary line.
func (s *StepLogger) Summary() {
	s.t.Helper()
	elapsed := time.Since(s.start).Round(time.Second)
	s.log("%s", strings.Repeat("=", lineWidth))
	s.log("  TEST COMPLETED in %s (%d steps)", elapsed, s.step)
	s.log("%s", strings.Repeat("=", lineWidth))
}

// Infof logs a formatted detail within the current step.
func (s *StepLogger) Infof(format string, args ...any) {
	s.t.Helper()
	s.log("  %s", fmt.Sprintf(format, args...))
}

// ---------------------------------------------------------------------------
// Per-test file logger
// ---------------------------------------------------------------------------

type fileLogger struct {
	mu   sync.Mutex
	file *os.File
}

func getOrCreateFileLogger(t *testing.T) *fileLogger {
	name := t.Name()
	if val, ok := testLoggers.Load(name); ok {
		return val.(*fileLogger)
	}

	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatalf("failed to create logs dir: %v", err)
	}

	logFile := filepath.Join(logsDir, name+".log")
	f, err := os.Create(logFile)
	if err != nil {
		t.Fatalf("failed to create log file %s: %v", logFile, err)
	}

	fl := &fileLogger{file: f}
	testLoggers.Store(name, fl)

	// Close the file when the test finishes.
	t.Cleanup(func() {
		fl.mu.Lock()
		defer fl.mu.Unlock()
		fl.file.Close()
	})

	return fl
}

func (fl *fileLogger) write(format string, args ...any) {
	fl.mu.Lock()
	defer fl.mu.Unlock()
	ts := time.Now().Format("15:04:05")
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(fl.file, "%s  %s\n", ts, msg)
}

// ---------------------------------------------------------------------------
// Consolidated report
// ---------------------------------------------------------------------------

// Fatalf logs a failure message to the per-test log file and then calls t.Fatalf.
// Use this instead of t.Fatalf to ensure failures appear in log files.
func Fatalf(t *testing.T, format string, args ...any) {
	t.Helper()
	msg := fmt.Sprintf(format, args...)
	Log(t, "  [FAIL] %s", msg)
	t.Fatalf("%s", msg)
}

// CleanLogDir removes all log files from previous runs.
// Call this from TestMain before m.Run().
func CleanLogDir() {
	os.RemoveAll(logsDir)
}

// PrintConsolidatedReport reads all per-test log files and prints them
// in a consolidated format to stdout. Call this from TestMain after m.Run().
func PrintConsolidatedReport() {
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return
	}

	fmt.Println()
	fmt.Println(strings.Repeat("=", lineWidth))
	fmt.Println("  CONSOLIDATED TEST REPORT")
	fmt.Println(strings.Repeat("=", lineWidth))

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(logsDir, entry.Name()))
		if err != nil {
			continue
		}

		testName := strings.TrimSuffix(entry.Name(), ".log")
		fmt.Printf("\n=== LOG  %s\n", testName)
		fmt.Print(string(data))
	}

	fmt.Println()
	fmt.Println(strings.Repeat("=", lineWidth))
}
