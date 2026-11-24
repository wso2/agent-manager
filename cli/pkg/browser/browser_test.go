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

package browser

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// Open launches the platform's URL handler. A fake handler first on PATH
// records its argument, proving the URL reaches it untouched.
func TestOpen_PassesURLToPlatformHandler(t *testing.T) {
	var handler string
	switch runtime.GOOS {
	case "darwin":
		handler = "open"
	case "linux":
		handler = "xdg-open"
	default:
		t.Skipf("no PATH-resolvable handler on %s", runtime.GOOS)
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "arg.txt")
	script := "#!/bin/sh\nprintf '%s' \"$1\" > '" + out + "'\n"
	if err := os.WriteFile(filepath.Join(dir, handler), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	const url = "https://idp.example.com/authorize?a=1&b=two words"
	if err := Open(url); err != nil {
		t.Fatalf("Open: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		got, err := os.ReadFile(out)
		if err == nil && len(got) > 0 {
			if string(got) != url {
				t.Errorf("handler got %q", got)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("handler was never run")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestOpen_MissingHandlerIsAnError(t *testing.T) {
	switch runtime.GOOS {
	case "darwin", "linux", "windows":
	default:
		t.Skip()
	}
	t.Setenv("PATH", t.TempDir()) // nothing to launch
	if err := Open("https://example.com"); err == nil {
		t.Error("Open must report that no browser launcher exists, so login prints the URL instead")
	}
}
