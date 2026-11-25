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

package tableprinter

import (
	"regexp"
	"strings"
	"testing"

	"github.com/wso2/agent-manager/cli/pkg/iostreams"
)

var ansi = regexp.MustCompile("\x1b\\[[0-9;]*m")

// plain drops the bold header styling a terminal gets.
func plain(s string) string { return ansi.ReplaceAllString(s, "") }

func printer(tty bool, headers ...string) (*TablePrinter, func() string) {
	ios, _, out, _ := iostreams.Test()
	ios.SetTerminal(tty, tty, tty)
	return New(ios, headers...), out.String
}

func TestRender_PipedOutputIsHeaderlessTSV(t *testing.T) {
	tp, out := printer(false, "name", "created")
	tp.AddField("alpha", WithColor(func(s string) string { return "<" + s + ">" }))
	tp.AddField("2026-01-02")
	tp.EndRow()
	tp.AddField("beta")
	tp.AddField("")
	tp.EndRow()
	if err := tp.Render(); err != nil {
		t.Fatal(err)
	}
	// Scripts parse this: no header, no padding, no colour.
	if got, want := out(), "alpha\t2026-01-02\nbeta\t\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRender_TerminalAlignsColumnsAndLeavesLastUnpadded(t *testing.T) {
	tp, out := printer(true, "name", "display name")
	tp.AddField("a")
	tp.AddField("Alpha")
	tp.EndRow()
	tp.AddField("longer-name")
	tp.AddField("B")
	tp.EndRow()
	if err := tp.Render(); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(plain(out()), "\n"), "\n")
	want := []string{
		"NAME         DISPLAY NAME",
		"a            Alpha",
		"longer-name  B",
	}
	if len(lines) != len(want) {
		t.Fatalf("lines = %q", lines)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, lines[i], want[i])
		}
	}
}

func TestRender_TerminalWithoutHeadersUsesRowWidth(t *testing.T) {
	tp, out := printer(true)
	tp.AddField("x")
	tp.AddField("y")
	tp.EndRow()
	tp.AddField("xxx")
	tp.AddField("z")
	tp.EndRow()
	if err := tp.Render(); err != nil {
		t.Fatal(err)
	}
	if got, want := out(), "x    y\nxxx  z\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRender_EmptyTableWritesNothing(t *testing.T) {
	tp, out := printer(true)
	if err := tp.Render(); err != nil || out() != "" {
		t.Errorf("err=%v out=%q", err, out())
	}
	// Headers alone still print on a terminal.
	tp2, out2 := printer(true, "name")
	_ = tp2.Render()
	if plain(out2()) != "NAME\n" {
		t.Errorf("header-only = %q", out2())
	}
}

func TestRender_ColorIsAppliedWithoutBreakingAlignment(t *testing.T) {
	ios, _, out, _ := iostreams.Test()
	ios.SetTerminal(true, true, true)
	tp := New(ios, "a", "b")
	tp.AddField("x", WithColor(func(s string) string { return "[" + s + "]" }))
	tp.AddField("1")
	tp.EndRow()
	tp.AddField("xyz")
	tp.AddField("2")
	tp.EndRow()
	_ = tp.Render()
	if !strings.Contains(out.String(), "[x]  ") || !strings.Contains(out.String(), "  1") {
		t.Errorf("out = %q", out.String())
	}
	// "x" is padded by its text width (3), not the width of the coloured string.
	if !strings.Contains(out.String(), "[x]    1") {
		t.Errorf("padding computed from coloured width: %q", out.String())
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errBroken }

var errBroken = &brokenErr{}

type brokenErr struct{}

func (*brokenErr) Error() string { return "broken pipe" }

func TestRender_TSVReportsWriteErrors(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	ios.Out = failingWriter{}
	tp := New(ios, "a")
	tp.AddField("x")
	tp.EndRow()
	if err := tp.Render(); err == nil || err.Error() != "broken pipe" {
		t.Errorf("err = %v", err)
	}
}
