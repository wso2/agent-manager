//go:build integration

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
package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestServeFromAmpYAMLAlone starts the amp binary with nothing but an
// amp.yaml: it must generate its secrets, start, sign the admin in, and reuse
// the same secrets on the next start.
func TestServeFromAmpYAMLAlone(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "amp")
	build := exec.Command("go", "build", "-o", bin, "./cmd/amp")
	build.Dir = ".."
	build.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	out, err := build.CombinedOutput()
	require.NoError(t, err, string(out))

	port := freePort(t)
	data := filepath.Join(dir, "data")
	cfg := filepath.Join(dir, "amp.yaml")
	require.NoError(t, os.WriteFile(cfg, []byte(fmt.Sprintf(
		"server: {port: %d, public_url: \"http://127.0.0.1:%d\"}\nauth: {mode: file}\namp: {data_dir: %q}\n",
		port, port, data,
	)), 0o600))
	base := fmt.Sprintf("http://127.0.0.1:%d", port)

	start := func() *exec.Cmd {
		cmd := exec.Command(bin, "serve", "--migrate")
		cmd.Dir = dir
		cmd.Env = []string{"AMP_CONFIG=" + cfg, "PATH=" + os.Getenv("PATH"), "HOME=" + dir}
		var logs bytes.Buffer
		cmd.Stdout, cmd.Stderr = &logs, &logs
		require.NoError(t, cmd.Start())
		t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })
		deadline := time.Now().Add(60 * time.Second)
		for {
			if resp, err := http.Get(base + "/healthz"); err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return cmd
				}
			}
			require.True(t, time.Now().Before(deadline), "amp serve did not become healthy:\n%s", logs.String())
			time.Sleep(500 * time.Millisecond)
		}
	}
	stop := func(cmd *exec.Cmd) {
		_ = cmd.Process.Signal(os.Interrupt)
		done := make(chan struct{})
		go func() { _, _ = cmd.Process.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(20 * time.Second):
			_ = cmd.Process.Kill()
		}
	}
	signIn := func() {
		pw, err := os.ReadFile(filepath.Join(data, "secrets", "admin-password"))
		require.NoError(t, err)
		body, _ := json.Marshal(map[string]string{"username": "admin", "password": strings.TrimSpace(string(pw))})
		resp, err := http.Post(base+"/auth/login", "application/json", bytes.NewReader(body))
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var tok struct {
			AccessToken string `json:"access_token"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&tok))
		require.NotEmpty(t, tok.AccessToken)
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, base+"/api/v1/ui-manifest", nil)
		req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
		got, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		_ = got.Body.Close()
		require.Equal(t, http.StatusOK, got.StatusCode)
	}

	first := start()
	signIn()
	key, err := os.ReadFile(filepath.Join(data, "secrets", "encryption-key"))
	require.NoError(t, err)
	stop(first)

	start()
	signIn()
	again, err := os.ReadFile(filepath.Join(data, "secrets", "encryption-key"))
	require.NoError(t, err)
	require.Equal(t, key, again, "the second start reuses the generated key")
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
