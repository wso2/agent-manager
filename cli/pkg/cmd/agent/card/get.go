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

package card

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cmd/agent/a2acard"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/render"
)

const (
	a2aSubType         = "a2a-agent"
	envNotFoundCode    = "ENVIRONMENT_NOT_FOUND"
	proxyStatusFailed  = "failed"
	notFetchedYetLabel = "(not fetched yet)"
)

// GetOptions holds the dependencies and resolved inputs for card get.
type GetOptions struct {
	IO           *iostreams.IOStreams
	Client       func(context.Context) (*amsvc.ClientWithResponses, error)
	ResolveScope func(*cobra.Command, bool, bool) (string, string, error)
	ResolveAgent func([]string) (string, []string, error)
	ResolveEnv   func(*cobra.Command) (string, error)
	MakeScope    func(org, proj, agent, env string) render.Scope

	Org       string
	Proj      string
	AgentName string
	Env       string
	Scope     render.Scope
}

// CardStatus is the --json payload of card get.
type CardStatus struct {
	Card           *amsvc.AgentCardResponse `json:"card"`                     // nil when no card row exists
	Proxy          *amsvc.A2AProxyResponse  `json:"proxy"`                    // nil when direct, internal or unknown
	RoutingUnknown bool                     `json:"routingUnknown,omitempty"` // caller may not read the routing
}

// NewGetCmd returns the "agent card get" command.
func NewGetCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &GetOptions{
		IO:           f.IOStreams,
		Client:       f.AgentManager,
		ResolveScope: f.ResolveOrgProject,
		ResolveAgent: f.ResolveAgent,
		ResolveEnv:   f.ResolveEnvironment,
		MakeScope:    f.EnvScope,
	}

	cmd := &cobra.Command{
		Use:   "get [agent]",
		Short: "Show an agent's card and routing in an environment",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, proj, err := opts.ResolveScope(cmd, true, true)
			if err != nil {
				return render.Error(opts.IO, render.Scope{}, err)
			}
			agentName, _, err := opts.ResolveAgent(args)
			if err != nil {
				return render.Error(opts.IO, render.Scope{}, err)
			}
			env, err := opts.ResolveEnv(cmd)
			if err != nil {
				return render.Error(opts.IO, render.Scope{}, err)
			}
			opts.Org, opts.Proj, opts.AgentName, opts.Env = org, proj, agentName, env
			opts.Scope = opts.MakeScope(org, proj, agentName, env)
			return runGet(cmd.Context(), opts)
		},
	}
	cmdutil.AddEnvFlag(cmd)
	cmd.ValidArgsFunction = completeAgentArg(f)
	return cmd
}

func runGet(ctx context.Context, o *GetOptions) error {
	if err := validateTarget(o.AgentName, o.Env); err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	client, err := o.Client(ctx)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}

	agentResp, err := client.GetAgentWithResponse(ctx, o.Org, o.Proj, o.AgentName)
	if err != nil {
		return render.Error(o.IO, o.Scope, clierr.Newf(clierr.Transport, "%v", err))
	}
	if agentResp.JSON200 == nil {
		return render.Error(o.IO, o.Scope, cmdutil.ErrorFromServer(agentResp.HTTPResponse,
			cmdutil.FirstNonNil(agentResp.JSON404, agentResp.JSON500)))
	}
	agent := agentResp.JSON200
	if agent.AgentType.SubType == nil || *agent.AgentType.SubType != a2aSubType {
		return render.Error(o.IO, o.Scope, clierr.Newf(clierr.Validation, "agent %q is not an A2A agent", o.AgentName))
	}
	external := agent.Provisioning.Type == amsvc.ProvisioningTypeExternal

	var status CardStatus
	if external {
		if status.Proxy, status.RoutingUnknown, err = fetchProxy(ctx, client, o); err != nil {
			return render.Error(o.IO, o.Scope, err)
		}
	} else if err := requireEnvironment(ctx, client, o); err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	if status.Card, err = fetchCard(ctx, client, o); err != nil {
		return render.Error(o.IO, o.Scope, err)
	}

	if o.IO.JSON {
		return render.JSONSuccess(o.IO, o.Scope, status)
	}
	printCardStatus(o.IO, o, external, status)
	return nil
}

// fetchProxy returns nil when the environment is direct, and unknown when the caller lacks the environment scope.
func fetchProxy(ctx context.Context, client *amsvc.ClientWithResponses, o *GetOptions) (proxy *amsvc.A2AProxyResponse, unknown bool, err error) {
	resp, err := client.GetA2AProxyWithResponse(ctx, o.Org, o.Proj, o.AgentName, o.Env)
	if err != nil {
		return nil, false, clierr.Newf(clierr.Transport, "%v", err)
	}
	switch {
	case resp.JSON200 != nil:
		return resp.JSON200, false, nil
	case resp.StatusCode() == http.StatusForbidden:
		return nil, true, nil
	case resp.StatusCode() == http.StatusNotFound && (resp.JSON404 == nil || resp.JSON404.Code != envNotFoundCode):
		return nil, false, nil
	}
	return nil, false, cmdutil.ErrorFromServer(resp.HTTPResponse,
		cmdutil.FirstNonNil(resp.JSON401, resp.JSON404, resp.JSON500))
}

// requireEnvironment fails on an unknown environment, which the card read alone reports as "no card yet".
func requireEnvironment(ctx context.Context, client *amsvc.ClientWithResponses, o *GetOptions) error {
	resp, err := client.GetEnvironmentWithResponse(ctx, o.Org, o.Env)
	if err != nil {
		return clierr.Newf(clierr.Transport, "%v", err)
	}
	switch resp.StatusCode() {
	case http.StatusOK, http.StatusForbidden:
		return nil // a forbidden lookup can't prove the env is missing, so the card read decides
	}
	return cmdutil.ErrorFromServer(resp.HTTPResponse,
		cmdutil.FirstNonNil(resp.JSON400, resp.JSON401, resp.JSON404, resp.JSON500))
}

// fetchCard returns nil when no card row exists.
func fetchCard(ctx context.Context, client *amsvc.ClientWithResponses, o *GetOptions) (*amsvc.AgentCardResponse, error) {
	resp, err := client.GetAgentCardWithResponse(ctx, o.Org, o.Proj, o.AgentName, o.Env)
	if err != nil {
		return nil, clierr.Newf(clierr.Transport, "%v", err)
	}
	switch {
	case resp.JSON200 != nil:
		return resp.JSON200, nil
	case resp.StatusCode() == http.StatusNotFound:
		return nil, nil
	}
	return nil, cmdutil.ErrorFromServer(resp.HTTPResponse,
		cmdutil.FirstNonNil(resp.JSON401, resp.JSON404, resp.JSON500))
}

func printCardStatus(ios *iostreams.IOStreams, o *GetOptions, external bool, s CardStatus) {
	w := ios.Out
	fmt.Fprintf(w, "Environment: %s\n", o.Env)
	if external {
		printRouting(w, s.Proxy, s.RoutingUnknown)
	}
	if s.Card == nil {
		if external {
			fmt.Fprintf(w, "no card URL registered; run: amctl agent card set %s --env %s --url <url>\n", o.AgentName, o.Env)
		} else {
			fmt.Fprintln(w, "no card fetched yet")
		}
		return
	}
	source := s.Card.SourceUrl
	if source == "" {
		source = string(s.Card.Source)
	}
	fmt.Fprintf(w, "Card source: %s\n", source)
	fmt.Fprintf(w, "Card status: %s\n", s.Card.Status)
	if s.Card.LastError != "" {
		fmt.Fprintf(w, "Card error: %s\n", s.Card.LastError)
	}
	fmt.Fprintf(w, "Card: %s\n", cardSummary(s.Card.Card))
}

func printRouting(w io.Writer, proxy *amsvc.A2AProxyResponse, unknown bool) {
	if unknown {
		fmt.Fprintln(w, "Routing: unknown (no permission to read gateway routing in this environment)")
		return
	}
	if proxy == nil {
		fmt.Fprintf(w, "Routing: %s\n", a2acard.RoutingDirect)
		return
	}
	fmt.Fprintf(w, "Routing: %s (%s)\n", a2acard.RoutingProxied, proxy.Status)
	if proxy.GatewayUrl == "" {
		fmt.Fprintln(w, "Gateway URL: no ingress gateway mapped")
	} else {
		fmt.Fprintf(w, "Gateway URL: %s\n", proxy.GatewayUrl)
	}
	if string(proxy.Status) == proxyStatusFailed {
		fmt.Fprintf(w, "Publish error: %s\n", proxy.LastError)
	}
}

// cardSummary reads untrusted card JSON defensively, skipping non-string fields.
func cardSummary(card *map[string]interface{}) string {
	if card == nil {
		return notFetchedYetLabel
	}
	stringField := func(key string) string {
		v, _ := (*card)[key].(string)
		return strings.Map(dropControl, v)
	}
	var parts []string
	if name := stringField("name"); name != "" {
		parts = append(parts, name)
	}
	if version := strings.TrimPrefix(stringField("version"), "v"); version != "" {
		parts = append(parts, "v"+version)
	}
	summary := strings.Join(parts, " ")
	if desc := stringField("description"); desc != "" {
		summary += " - " + desc
	}
	return strings.TrimPrefix(summary, " - ")
}

// dropControl removes C0/C1 control runes so third-party card text can't inject terminal escapes.
func dropControl(r rune) rune {
	if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
		return -1
	}
	return r
}
