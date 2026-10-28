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

// Package a2acard holds the card-source flow shared by agent card set and agent create.
package a2acard

import (
	"context"
	"net/http"
	"net/url"
	"path"
	"strings"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
)

// Routing values describe how an environment's card is reached.
const (
	RoutingDirect  = "direct"
	RoutingProxied = "proxied"
)

const wellKnownCardPath = "/.well-known/agent-card.json"

// Target identifies the agent environment whose card source is set.
type Target struct{ Org, Proj, Agent, Env string }

// SourceResult reports the saved card URL and how it is routed.
type SourceResult struct {
	Environment string `json:"environment"`
	URL         string `json:"url"`
	Routing     string `json:"routing"` // RoutingDirect, RoutingProxied, or "" when unknown
}

// CheckProxyableURL refuses, before anything is saved, a URL the server's proxy step would refuse.
func CheckProxyableURL(flag, raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return cmdutil.FlagErrorf("%s must be an http(s) URL", flag)
	}
	if u.RawQuery != "" || u.Fragment != "" || strings.Contains(raw, "?") || strings.Contains(raw, "#") {
		return cmdutil.FlagErrorf("%s must not have a query or fragment to use --proxy", flag)
	}
	if !strings.HasSuffix(u.Path, wellKnownCardPath) {
		return cmdutil.FlagErrorf("%s must end in %s to use --proxy", flag, wellKnownCardPath)
	}
	base := strings.TrimSuffix(u.Path, wellKnownCardPath)
	if base != "" && (strings.HasSuffix(base, "/") || path.Clean(base) != base || u.RawPath != "") {
		return cmdutil.FlagErrorf("%s must have a canonical path (no \"//\", \"..\", trailing \"/\" or escaped characters before %s) to use --proxy", flag, wellKnownCardPath)
	}
	return nil
}

// ApplySource saves the card URL and, when proxy is set, routes the card through the gateway.
func ApplySource(ctx context.Context, client *amsvc.ClientWithResponses, t Target, cardURL string, proxy bool) (SourceResult, error) {
	res := SourceResult{Environment: t.Env, URL: cardURL}

	src, err := client.SetAgentCardSourceWithResponse(ctx, t.Org, t.Proj, t.Agent, t.Env, amsvc.SetAgentCardSourceRequest{Url: cardURL})
	if err != nil {
		return res, clierr.Newf(clierr.Transport, "%v", err)
	}
	if src.StatusCode() != http.StatusAccepted {
		return res, sourceError(src)
	}

	cur, err := client.GetA2AProxyWithResponse(ctx, t.Org, t.Proj, t.Agent, t.Env)
	if err != nil {
		return res, partialFailure(res, clierr.Newf(clierr.Transport, "%v", err))
	}
	switch cur.StatusCode() {
	case http.StatusOK:
		res.Routing = RoutingProxied
	case http.StatusNotFound:
		res.Routing = RoutingDirect
	default:
		return res, partialFailure(res, cmdutil.ErrorFromServer(cur.HTTPResponse,
			cmdutil.FirstNonNil(cur.JSON401, cur.JSON404, cur.JSON500)))
	}
	if !proxy || res.Routing == RoutingProxied {
		return res, nil
	}

	put, err := client.SetA2AProxyWithResponse(ctx, t.Org, t.Proj, t.Agent, t.Env, amsvc.SetA2AProxyRequest{})
	if err != nil {
		return res, partialFailure(res, clierr.Newf(clierr.Transport, "%v", err))
	}
	if put.StatusCode() != http.StatusAccepted {
		return res, partialFailure(res, cmdutil.ErrorFromServer(put.HTTPResponse,
			cmdutil.FirstNonNil(put.JSON400, put.JSON403, put.JSON404, put.JSON409, put.JSON500, put.JSON503)))
	}
	res.Routing = RoutingProxied
	return res, nil
}

func sourceError(src *amsvc.SetAgentCardSourceResp) clierr.CLIError {
	e := cmdutil.ErrorFromServer(src.HTTPResponse,
		cmdutil.FirstNonNil(src.JSON400, src.JSON403, src.JSON404, src.JSON409, src.JSON500))
	if strings.Contains(e.Message, "publicly reachable") {
		e.Message += "; expose the agent on a public URL (e.g. a tunnel)"
	}
	if src.StatusCode() == http.StatusConflict {
		e.Message += " (only the console can clear it)"
	}
	return e
}

// partialFailure reports a failure after the URL was already saved.
func partialFailure(res SourceResult, cause clierr.CLIError) clierr.CLIError {
	data := make(map[string]any, len(cause.AdditionalData)+4)
	for k, v := range cause.AdditionalData {
		data[k] = v
	}
	data["environment"] = res.Environment
	data["url"] = res.URL
	data["routing"] = res.Routing
	data["urlSaved"] = true
	cause.Message = "card URL saved for " + res.Environment + ", but " + cause.Message
	cause.AdditionalData = data
	return cause
}
