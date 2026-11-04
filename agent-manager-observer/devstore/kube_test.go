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
//go:build devstore

package devstore

import (
	"os"
	"path/filepath"
	"testing"
)

const testKubeconfig = `apiVersion: v1
kind: Config
clusters:
- name: c
  cluster: {server: "https://127.0.0.1:6443"}
users:
- name: u
  user: {token: t}
contexts:
- name: x
  context: {cluster: c, user: u}
current-context: x
`

func useTestKubeconfig(t *testing.T) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(path, []byte(testKubeconfig), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", path)
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
}

func TestKubeConfigRaisesClientRateLimits(t *testing.T) {
	useTestKubeconfig(t)

	cfg, err := kubeConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.QPS != 50 || cfg.Burst != 100 {
		t.Fatalf("QPS=%v Burst=%d, want 50 and 100", cfg.QPS, cfg.Burst)
	}
}

func TestNewKubeClientsBuildsBothClients(t *testing.T) {
	useTestKubeconfig(t)

	dyn, typed, err := newKubeClients()
	if err != nil {
		t.Fatal(err)
	}
	if dyn == nil || typed == nil {
		t.Fatalf("dynamic=%v typed=%v, want both set", dyn, typed)
	}
}
