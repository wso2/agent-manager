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

// Package environment provides e2e operations for managing environments,
// including running the operator-facing add/remove environment shell scripts
// that provision the API Platform Gateway via helm.
package environment

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// ScriptParams are the inputs for the add/remove environment scripts.
type ScriptParams struct {
	OrgName      string
	EnvName      string
	DisplayName  string
	Token        string
	AMPBaseURL   string
	IsProduction bool

	// Topology overrides the ambient GATEWAY_TOPOLOGY env var when set (e.g.
	// "split" to provision separate INGRESS/EGRESS gateways). Leave blank to
	// use the ambient value, defaulting to "single".
	Topology string

	// ProvisionThunder forces PROVISION_THUNDER=true for this run, overriding the
	// ambient value. AddEnvironment then fails unless that Thunder is registered.
	ProvisionThunder bool

	// Timeout bounds the whole script run (helm install + kubectl wait can be slow).
	// Defaults to 6 minutes when zero, or 12 minutes with ProvisionThunder.
	Timeout time.Duration

	// client is set by FromClient; AddEnvironment uses it to verify ProvisionThunder.
	client *framework.AMPClient
}

// repoRoot resolves the checkout root relative to this source file, so repo paths
// can be located regardless of the working directory ginkgo runs from. This file
// lives at <repo>/test/e2e/operations/environment/scripts.go, so the repo root is
// four directories up.
func repoRoot() string {
	_, thisFile, _, ok := runtime.Caller(0)
	Expect(ok).To(BeTrue(), "failed to resolve caller path for script location")
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..")
}

func repoScriptsDir() string {
	return filepath.Join(repoRoot(), "deployments", "scripts")
}

// gatewayChart returns GATEWAY_CHART with a repo-relative chart directory made
// absolute. The script runs from the test package's directory, where a path like
// deployments/helm-charts/... resolves nowhere. Anything that isn't a directory in
// the checkout (an oci:// ref, a helm repo alias) passes through unchanged.
func gatewayChart() string {
	chart := strings.TrimSpace(os.Getenv("GATEWAY_CHART"))
	if chart == "" || filepath.IsAbs(chart) {
		return chart
	}
	if local := filepath.Join(repoRoot(), chart); isDir(local) {
		return local
	}
	return chart
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// fileURL returns a percent-escaped file:// URL for an absolute path, so a
// checkout path containing spaces still works with curl.
func fileURL(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

// runScript executes the named script with the script-specific env vars layered
// on top of the ambient environment. It returns an error rather than asserting,
// so callers can choose to fail hard (tests) or log-and-continue (cleanup).
func runScript(scriptName string, params *ScriptParams, extraEnv []string) error {
	timeout := params.Timeout
	if timeout == 0 {
		timeout = 6 * time.Minute
		if params.ProvisionThunder {
			timeout = 12 * time.Minute
		}
	}
	scriptPath := filepath.Join(repoScriptsDir(), scriptName)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	env := append([]string{}, extraEnv...)
	env = append(env,
		"ENV_NAME="+params.EnvName,
		"AGENT_MANAGER_TOKEN="+params.Token,
		"ORG_NAME="+params.OrgName,
		"AGENT_MANAGER_URL="+params.AMPBaseURL,
		"SCRIPT_BASE_URL="+envOr("SCRIPT_BASE_URL", fileURL(repoScriptsDir())),
	)

	cmd := exec.CommandContext(ctx, "bash", scriptPath)
	// Inherit the ambient environment (PATH, KUBECONFIG, etc.) and layer the
	// script-specific variables on top.
	cmd.Env = append(cmd.Environ(), env...)
	// One shared writer, so exec never calls Write from two goroutines at once.
	out := &redactingWriter{w: ginkgo.GinkgoWriter}
	cmd.Stdout = out
	cmd.Stderr = out

	ginkgo.GinkgoWriter.Printf("Running %s for env %q...\n", scriptName, params.EnvName)
	err := cmd.Run()
	out.flush()
	return err
}

// passwordLine matches the admin password add-environment-thunder.sh prints
// for a new env-Thunder ("  Password: <value>").
var passwordLine = regexp.MustCompile(`(?i)^(\s*password:\s*)\S.*$`)

// redactingWriter forwards script output line by line, replacing password
// values so they never reach GinkgoWriter (and from there, CI logs).
type redactingWriter struct {
	w   io.Writer
	buf []byte
}

func (r *redactingWriter) Write(p []byte) (int, error) {
	r.buf = append(r.buf, p...)
	for {
		i := bytes.IndexByte(r.buf, '\n')
		if i < 0 {
			return len(p), nil
		}
		if _, err := r.w.Write(redactLine(r.buf[:i+1])); err != nil {
			return len(p), err
		}
		r.buf = r.buf[i+1:]
	}
}

// flush writes any trailing output that did not end in a newline.
func (r *redactingWriter) flush() {
	if len(r.buf) > 0 {
		_, _ = r.w.Write(redactLine(r.buf))
		r.buf = nil
	}
}

func redactLine(line []byte) []byte {
	body, hadNewline := bytes.CutSuffix(line, []byte("\n"))
	out := passwordLine.ReplaceAll(body, []byte("${1}<redacted>"))
	if hadNewline {
		out = append(out, '\n')
	}
	return out
}

// envOr returns the ambient value of key when set (non-blank), else fallback.
func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// AddEnvironment runs deployments/scripts/add-environment.sh, which creates the
// environment via the Agent Manager API and installs its API Platform Gateway.
// Asserts the script succeeds.
func AddEnvironment(params *ScriptParams) {
	AddEnvironmentWithContext(context.Background(), params)
}

// AddEnvironmentWithContext is AddEnvironment with a caller-supplied context
// (e.g. a Ginkgo SpecContext) that cancels the ProvisionThunder readiness poll.
func AddEnvironmentWithContext(ctx context.Context, params *ScriptParams) {
	chartVersion := strings.TrimSpace(os.Getenv("CHART_VERSION"))
	Expect(chartVersion).NotTo(BeEmpty(),
		"CHART_VERSION must be set for add-environment.sh; set it to the chart version "+
			"the platform was installed with (see test/e2e/.env.example)")

	topology := params.Topology
	if topology == "" {
		topology = envOr("GATEWAY_TOPOLOGY", "single")
	}

	extra := []string{
		"DISPLAY_NAME=" + params.DisplayName,
		"CHART_VERSION=" + chartVersion,
		fmt.Sprintf("IS_PRODUCTION=%t", params.IsProduction),
		"GATEWAY_TOPOLOGY=" + topology,
		"AGENT_MANAGER_INTERNAL_BASE_URL=" + envOr("AGENT_MANAGER_INTERNAL_BASE_URL",
			"http://amp-api.wso2-amp.svc.cluster.local:9000"),
		"AGENT_MANAGER_INTERNAL_CP=" + envOr("AGENT_MANAGER_INTERNAL_CP",
			"amp-api-gateway-manager.wso2-amp.svc.cluster.local:9243"),
	}
	if chart := gatewayChart(); chart != "" {
		extra = append(extra, "GATEWAY_CHART="+chart)
	}
	if params.ProvisionThunder {
		extra = append(extra, "PROVISION_THUNDER=true")
	}
	err := runScript("add-environment.sh", params, extra)
	Expect(err).NotTo(HaveOccurred(), "add-environment.sh failed for env %q", params.EnvName)
	if params.ProvisionThunder {
		expectDedicatedThunder(ctx, params)
	}
}

// expectDedicatedThunder fails unless amp-api lists the env's own Thunder.
func expectDedicatedThunder(ctx context.Context, params *ScriptParams) {
	Expect(params.client).NotTo(BeNil(), "ProvisionThunder needs ScriptParams built by FromClient")
	Eventually(func(g Gomega) {
		list := ListThunderInstances(ctx, g, params.client, params.OrgName)
		var envs []string
		for _, ti := range list.ThunderInstances {
			envs = append(envs, ti.EnvName)
		}
		g.Expect(envs).To(ContainElement(params.EnvName))
	}).WithContext(ctx).WithTimeout(2*time.Minute).WithPolling(5*time.Second).Should(Succeed(),
		"no dedicated Thunder registered and reachable for env %q; see the add-environment.sh output above", params.EnvName)
}

// RemoveEnvironment runs deployments/scripts/remove-environment.sh, which
// uninstalls the gateway helm release and deletes the environment. It is
// best-effort: it returns any error instead of asserting, so the stale-resource
// sweep can log-and-continue rather than abort.
func RemoveEnvironment(params *ScriptParams) error {
	return runScript("remove-environment.sh", params, nil)
}

// FromClient builds ScriptParams from the test client/config, filling in the
// org, base URL and bearer token. The caller sets EnvName/DisplayName.
func FromClient(client *framework.AMPClient) *ScriptParams {
	cfg := client.Cfg()
	return &ScriptParams{
		OrgName:    cfg.DefaultOrg,
		AMPBaseURL: cfg.AMPBaseURL,
		Token:      client.Token(),
		client:     client,
	}
}
