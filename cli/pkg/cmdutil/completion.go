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

package cmdutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
)

const completionTimeout = 2 * time.Second

// userCacheDir is overridable in tests. Defaults to os.UserCacheDir which
// returns the platform-native cache dir (~/.cache on Linux, ~/Library/Caches
// on macOS).
var userCacheDir = os.UserCacheDir

// CompleteInstances returns sorted instance names from local config.
// Returns nil if config can't be loaded. No API calls.
func CompleteInstances(f *Factory) []string {
	cfg, err := f.Config()
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(cfg.Instances))
	for name := range cfg.Instances {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// logCompletionErr appends one line to <userCacheDir>/amctl/completion.log
// when AMCTL_COMPLETION_DEBUG=1. Silent and best-effort: any IO error is
// ignored so a broken log path never disrupts a TAB completion.
func logCompletionErr(op string, kv map[string]string, err error) {
	if err == nil || os.Getenv("AMCTL_COMPLETION_DEBUG") != "1" {
		return
	}
	cacheDir, cerr := userCacheDir()
	if cerr != nil {
		return
	}
	dir := filepath.Join(cacheDir, "amctl")
	if mkerr := os.MkdirAll(dir, 0o700); mkerr != nil {
		return
	}
	logPath := filepath.Join(dir, "completion.log")
	f, oerr := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if oerr != nil {
		return
	}
	defer f.Close()

	keys := make([]string, 0, len(kv))
	for k := range kv {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+kv[k])
	}
	pairs := strings.Join(parts, " ")
	if pairs != "" {
		pairs = " " + pairs
	}
	fmt.Fprintf(f, "%s %s%s err=%v\n", time.Now().UTC().Format(time.RFC3339), op, pairs, err)
}

// CompleteProjects returns sorted project names in the resolved org.
// Org resolution follows ResolveOrgProject(cmd, true, false): --org flag,
// then instance.CurrentOrg. Returns nil if org cannot be resolved or on
// any API error.
func CompleteProjects(cmd *cobra.Command, f *Factory) []string {
	org, _, err := f.ResolveOrgProject(cmd, true, false)
	if err != nil {
		logCompletionErr("CompleteProjects", nil, err)
		return nil
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), completionTimeout)
	defer cancel()

	client, err := f.AgentManager(ctx)
	if err != nil {
		logCompletionErr("CompleteProjects", map[string]string{"org": org}, err)
		return nil
	}
	resp, err := client.ListProjectsWithResponse(ctx, org, &amsvc.ListProjectsParams{})
	if err != nil {
		logCompletionErr("CompleteProjects", map[string]string{"org": org}, err)
		return nil
	}
	if resp.JSON200 == nil {
		logCompletionErr("CompleteProjects", map[string]string{"org": org}, fmt.Errorf("status %d", resp.StatusCode()))
		return nil
	}
	out := make([]string, 0, len(resp.JSON200.Projects))
	for _, p := range resp.JSON200.Projects {
		out = append(out, p.Name)
	}
	sort.Strings(out)
	return out
}

// CompleteAgents returns sorted agent names in the resolved (org, project).
// Both org and project must be resolvable; otherwise returns nil.
func CompleteAgents(cmd *cobra.Command, f *Factory) []string {
	org, proj, err := f.ResolveOrgProject(cmd, true, true)
	if err != nil {
		logCompletionErr("CompleteAgents", nil, err)
		return nil
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), completionTimeout)
	defer cancel()

	client, err := f.AgentManager(ctx)
	if err != nil {
		logCompletionErr("CompleteAgents", map[string]string{"org": org, "project": proj}, err)
		return nil
	}
	resp, err := client.ListAgentsWithResponse(ctx, org, proj, &amsvc.ListAgentsParams{})
	if err != nil {
		logCompletionErr("CompleteAgents", map[string]string{"org": org, "project": proj}, err)
		return nil
	}
	if resp.JSON200 == nil {
		logCompletionErr("CompleteAgents", map[string]string{"org": org, "project": proj}, fmt.Errorf("status %d", resp.StatusCode()))
		return nil
	}
	out := make([]string, 0, len(resp.JSON200.Agents))
	for _, a := range resp.JSON200.Agents {
		out = append(out, a.Name)
	}
	sort.Strings(out)
	return out
}

// CompleteOrgs returns sorted organization names. Returns nil on any error
// (no instance, network, server). Times out at 2s.
func CompleteOrgs(cmd *cobra.Command, f *Factory) []string {
	ctx, cancel := context.WithTimeout(cmd.Context(), completionTimeout)
	defer cancel()

	client, err := f.AgentManager(ctx)
	if err != nil {
		logCompletionErr("CompleteOrgs", nil, err)
		return nil
	}
	resp, err := client.ListOrganizationsWithResponse(ctx, &amsvc.ListOrganizationsParams{})
	if err != nil {
		logCompletionErr("CompleteOrgs", nil, err)
		return nil
	}
	if resp.JSON200 == nil {
		logCompletionErr("CompleteOrgs", nil, fmt.Errorf("status %d", resp.StatusCode()))
		return nil
	}
	out := make([]string, 0, len(resp.JSON200.Organizations))
	for _, org := range resp.JSON200.Organizations {
		out = append(out, org.Name)
	}
	sort.Strings(out)
	return out
}
