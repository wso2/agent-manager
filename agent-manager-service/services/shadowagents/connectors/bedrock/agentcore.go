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

package bedrock

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	ac "github.com/aws/aws-sdk-go-v2/service/bedrockagentcorecontrol"
	actypes "github.com/aws/aws-sdk-go-v2/service/bedrockagentcorecontrol/types"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
)

// scanRuntimes lists the region's AgentCore runtimes (evidence §2.1) and
// reads each one (§2.2); a runtime that hosts an MCP server is left out. It
// returns the listing error, so the caller can stop on an authentication
// failure.
func (s *scan) scanRuntimes(ctx context.Context, rc *regionClients) error {
	p := ac.NewListAgentRuntimesPaginator(rc.agentCore, &ac.ListAgentRuntimesInput{MaxResults: aws.Int32(pageSize), NextToken: nil})
	var ids []string
	n, err := paginate(ctx, maxPages, p.HasMorePages, func(ctx context.Context) (int, error) {
		out, err := p.NextPage(ctx)
		if err != nil {
			return 0, err
		}
		for _, rt := range out.AgentRuntimes {
			// The docs do not say whether the list has one row per runtime or
			// per version; dedupe on the runtime ID (evidence §2.1).
			id := str(rt.AgentRuntimeId)
			if id == "" || s.runtimeByID[rc.region+"|"+id] != nil {
				continue
			}
			f := s.runtimeFromSummary(rc.region, rt)
			if f == nil {
				continue
			}
			ids = append(ids, id)
		}
		return len(out.AgentRuntimes), nil
	})
	s.calls.Record(callListRuntimes, n, err)
	if isAuthFailure(err) {
		return err
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			break
		}
		s.getRuntime(ctx, rc, id)
	}
	return err
}

func (s *scan) runtimeFromSummary(region string, rt actypes.AgentRuntime) *sa.Finding {
	arn := str(rt.AgentRuntimeArn)
	if arn == "" {
		return nil
	}
	f := s.fs.Get(models.ShadowKindRuntime, arn)
	f.DisplayName = sa.SanitizeName(str(rt.AgentRuntimeName))
	f.Scope = region
	f.State = string(rt.Status)
	f.Attributes["region"] = region
	f.Attributes["resourceId"] = arn
	f.Attributes["agentVersion"] = str(rt.AgentRuntimeVersion)
	f.Attributes["status"] = string(rt.Status)
	// description is @sensitive in the model (evidence §2.1): never stored.
	f.AddSignal(sa.Signal{Code: sa.SigBedrockAgentCoreRuntime, Source: callListRuntimes, Observed: map[string]any{"status": string(rt.Status)}})
	s.runtimeByARN[arn] = f
	s.runtimeByID[region+"|"+str(rt.AgentRuntimeId)] = f
	return f
}

// getRuntime adds the execution role, protocol and artifact of a runtime.
// environmentVariables (a @sensitive map), description, authorizer and
// network settings are never read into the finding.
func (s *scan) getRuntime(ctx context.Context, rc *regionClients, id string) {
	out, err := rc.agentCore.GetAgentRuntime(ctx, &ac.GetAgentRuntimeInput{AgentRuntimeId: aws.String(id), AgentRuntimeVersion: nil})
	if err != nil {
		s.calls.Record(callGetRuntime, 0, awsError(err))
		return
	}
	s.calls.Record(callGetRuntime, 1, nil)
	f := s.runtimeByID[rc.region+"|"+id]
	if f == nil {
		return
	}
	if out.ProtocolConfiguration != nil && out.ProtocolConfiguration.ServerProtocol == actypes.ServerProtocolMcp {
		// A runtime can host an MCP server: that is a tool agents call, not
		// an agent, so it is not a finding.
		s.mcpRuntimes[f.ExternalID] = true
		s.fs.Remove(f.Kind, f.ExternalID)
		return
	}
	if role := str(out.RoleArn); role != "" {
		f.Attributes["roleArn"] = role
	}
	if out.ProtocolConfiguration != nil && out.ProtocolConfiguration.ServerProtocol != "" {
		f.Attributes["serverProtocol"] = string(out.ProtocolConfiguration.ServerProtocol)
	}
	switch a := out.AgentRuntimeArtifact.(type) {
	case *actypes.AgentRuntimeArtifactMemberContainerConfiguration:
		f.Attributes["agentKind"] = "container"
		f.Attributes["image"] = str(a.Value.ContainerUri)
	case *actypes.AgentRuntimeArtifactMemberCodeConfiguration:
		// Code deployments (S3 bucket, entry point, language runtime) have
		// no allowed attribute beyond the artifact kind.
		f.Attributes["agentKind"] = "code"
	}
	f.ProviderCreatedAt = timePtr(out.CreatedAt)
}
