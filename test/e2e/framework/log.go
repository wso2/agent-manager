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
	"strings"
	"testing"
	"time"
)

const lineWidth = 70

// StepLogger provides structured, readable logging for integration test steps.
type StepLogger struct {
	t     *testing.T
	step  int
	start time.Time
}

// NewStepLogger creates a new step logger for the given test.
func NewStepLogger(t *testing.T) *StepLogger {
	return &StepLogger{t: t, start: time.Now()}
}

// TestHeader prints the test name as a prominent header.
func (s *StepLogger) TestHeader(name string) {
	s.t.Helper()
	s.t.Logf("\n%s", strings.Repeat("=", lineWidth))
	s.t.Logf("  TEST: %s", name)
	s.t.Logf("%s", strings.Repeat("=", lineWidth))
}

// Begin starts a new numbered step with a header.
func (s *StepLogger) Begin(title string) {
	s.t.Helper()
	s.step++
	s.t.Logf("\n%s", strings.Repeat("-", lineWidth))
	s.t.Logf("  STEP %d: %s", s.step, title)
	s.t.Logf("%s", strings.Repeat("-", lineWidth))
}

// Info logs a key-value detail within the current step.
func (s *StepLogger) Info(key, value string) {
	s.t.Helper()
	s.t.Logf("  %-12s %s", key+":", value)
}

// Done marks the current step as complete with elapsed time.
func (s *StepLogger) Done(msg string, since time.Time) {
	s.t.Helper()
	elapsed := time.Since(since).Round(time.Millisecond)
	s.t.Logf("  [PASS] %s (%s)", msg, elapsed)
}

// Summary prints a final summary line.
func (s *StepLogger) Summary() {
	s.t.Helper()
	elapsed := time.Since(s.start).Round(time.Second)
	s.t.Logf("\n%s", strings.Repeat("=", lineWidth))
	s.t.Logf("  TEST COMPLETED in %s (%d steps)", elapsed, s.step)
	s.t.Logf("%s", strings.Repeat("=", lineWidth))
}

// Infof logs a formatted detail within the current step.
func (s *StepLogger) Infof(format string, args ...any) {
	s.t.Helper()
	s.t.Logf("  %s", fmt.Sprintf(format, args...))
}
