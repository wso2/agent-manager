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

package amcmd

import (
	"io"
	"os"
	"strings"
	"testing"
)

// runMain runs Main with the given args against an isolated HOME and returns
// the exit code with everything written to stdout and stderr.
func runMain(t *testing.T, home string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	origArgs, origOut, origErr := os.Args, os.Stdout, os.Stderr
	t.Cleanup(func() { os.Args, os.Stdout, os.Stderr = origArgs, origOut, origErr })

	outR, outW, _ := os.Pipe()
	errR, errW, _ := os.Pipe()
	os.Args, os.Stdout, os.Stderr = append([]string{"amctl"}, args...), outW, errW

	code = Main()

	_ = outW.Close()
	_ = errW.Close()
	o, _ := io.ReadAll(outR)
	e, _ := io.ReadAll(errR)
	return code, string(o), string(e)
}

func TestMain_Version(t *testing.T) {
	code, out, _ := runMain(t, t.TempDir(), "version")
	if code != 0 || !strings.HasPrefix(out, "amctl version ") {
		t.Errorf("code=%d out=%q", code, out)
	}
}

func TestMain_ExitCodes(t *testing.T) {
	home := t.TempDir()

	// Bad flag: usage error => 2, message and usage on stderr.
	code, _, errOut := runMain(t, home, "version", "--bogus")
	if code != 2 {
		t.Errorf("bad flag exit code = %d, want 2", code)
	}
	if !strings.Contains(errOut, "unknown flag: --bogus") || !strings.Contains(errOut, "Usage:") {
		t.Errorf("stderr = %q", errOut)
	}

	// Runtime failure (no instance configured): 1, rendered once, no usage dump.
	code, _, errOut = runMain(t, home, "project", "list", "--org", "acme")
	if code != 1 {
		t.Errorf("runtime failure exit code = %d, want 1", code)
	}
	if strings.Count(errOut, "no instance") == 0 || strings.Contains(errOut, "Usage:") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestMain_JSONErrorEnvelopeHasNoUsage(t *testing.T) {
	code, out, errOut := runMain(t, t.TempDir(), "project", "list", "--org", "acme", "--json")
	if code != 1 {
		t.Errorf("exit code = %d", code)
	}
	combined := out + errOut
	if !strings.Contains(combined, `"code": "NO_INSTANCE"`) && !strings.Contains(combined, `"code":"NO_INSTANCE"`) {
		t.Errorf("no JSON error envelope: %q", combined)
	}
	if strings.Contains(combined, "Usage:") {
		t.Errorf("usage leaked into JSON mode: %q", combined)
	}
}

func TestMain_UnknownCommandIsAnErrorWithUsage(t *testing.T) {
	code, _, errOut := runMain(t, t.TempDir(), "frobnicate")
	if code != 1 || !strings.Contains(errOut, "frobnicate") {
		t.Errorf("code=%d stderr=%q", code, errOut)
	}
}

func TestMain_UnreadableConfigFails(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(home+"/.amctl", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(home+"/.amctl/config", []byte("instances: [not, a, map"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := runMain(t, home, "version")
	if code != 1 || !strings.Contains(errOut, "yaml") {
		t.Errorf("code=%d stderr=%q", code, errOut)
	}
}
