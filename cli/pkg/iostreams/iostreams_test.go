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

package iostreams

import (
	"strings"
	"testing"
)

func TestTest_CapturesAndIsNotInteractive(t *testing.T) {
	ios, in, out, errOut := Test()
	in.WriteString("typed")
	ios.Out.Write([]byte("o"))
	ios.ErrOut.Write([]byte("e"))
	if out.String() != "o" || errOut.String() != "e" {
		t.Errorf("out=%q err=%q", out, errOut)
	}
	if ios.CanPrompt() || ios.IsStdoutTTY() || ios.ColorScheme().Enabled || ios.StderrColorScheme().Enabled {
		t.Error("test streams must behave like a pipe: no prompts, no colour")
	}
}

func TestSetTerminal_DrivesPromptAndColour(t *testing.T) {
	ios, _, _, _ := Test()

	ios.SetTerminal(true, false, true)
	if !ios.CanPrompt() {
		t.Error("stdin and stderr terminals => can prompt (prompts go to stderr)")
	}
	if ios.IsStdoutTTY() || ios.ColorScheme().Enabled {
		t.Error("stdout is piped: no colour on stdout")
	}
	if !ios.StderrColorScheme().Enabled {
		t.Error("stderr is a terminal: colour on stderr")
	}

	ios.SetTerminal(true, true, false)
	if ios.CanPrompt() {
		t.Error("stderr piped => cannot show a prompt")
	}
	if !ios.IsStdoutTTY() || !ios.ColorScheme().Enabled {
		t.Error("stdout terminal => colour")
	}
}

func TestSystem_WiresProcessStreams(t *testing.T) {
	ios := System()
	if ios.In == nil || ios.Out == nil || ios.ErrOut == nil || ios.JSON {
		t.Errorf("System() = %+v", ios)
	}
}

func TestColorScheme(t *testing.T) {
	on, off := &ColorScheme{Enabled: true}, &ColorScheme{}
	for name, fn := range map[string]func(*ColorScheme, string) string{
		"bold":   (*ColorScheme).Bold,
		"red":    (*ColorScheme).Red,
		"green":  (*ColorScheme).Green,
		"yellow": (*ColorScheme).Yellow,
		"cyan":   (*ColorScheme).Cyan,
		"gray":   (*ColorScheme).Gray,
		"header": (*ColorScheme).TableHeader,
	} {
		if got := fn(off, "x"); got != "x" {
			t.Errorf("%s disabled = %q", name, got)
		}
		got := fn(on, "x")
		if !strings.HasPrefix(got, "\033[") || !strings.HasSuffix(got, ansiReset) || !strings.Contains(got, "x") {
			t.Errorf("%s enabled = %q", name, got)
		}
	}
	if off.SuccessIcon() != "✓" || off.FailureIcon() != "X" {
		t.Errorf("icons = %q %q", off.SuccessIcon(), off.FailureIcon())
	}
	if on.SuccessIcon() == "✓" || on.FailureIcon() == "X" {
		t.Error("icons should be coloured when enabled")
	}
}
