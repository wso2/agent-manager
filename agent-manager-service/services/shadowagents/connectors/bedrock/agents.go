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
	ba "github.com/aws/aws-sdk-go-v2/service/bedrockagent"
	batypes "github.com/aws/aws-sdk-go-v2/service/bedrockagent/types"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
)

// draftVersion is the working version every classic agent has; its action
// groups are the agent's current tools (evidence §2.7).
const draftVersion = "DRAFT"

// classicAgentExternalID is stable whether or not GetAgent (which returns
// the ARN) succeeds.
func classicAgentExternalID(region, agentID string) string {
	return "bedrock:" + region + ":agent/" + agentID
}

// scanClassicAgents lists the region's classic Bedrock agents (evidence
// §2.6), reads each one and lists its action groups (§2.7).
func (s *scan) scanClassicAgents(ctx context.Context, rc *regionClients) {
	p := ba.NewListAgentsPaginator(rc.agents, &ba.ListAgentsInput{MaxResults: aws.Int32(pageSize), NextToken: nil})
	var ids []string
	n, err := paginate(ctx, maxPages, p.HasMorePages, func(ctx context.Context) (int, error) {
		out, err := p.NextPage(ctx)
		if err != nil {
			return 0, err
		}
		for _, a := range out.AgentSummaries {
			if f := s.classicAgentFromSummary(rc.region, a); f != nil {
				ids = append(ids, str(a.AgentId))
			}
		}
		return len(out.AgentSummaries), nil
	})
	s.calls.Record(callListAgents, n, err)
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		f, _ := s.fs.Lookup(models.ShadowKindPlatformAgent, classicAgentExternalID(rc.region, id))
		s.getClassicAgent(ctx, rc, id, f)
		s.actionGroups(ctx, rc, id, f)
	}
}

func (s *scan) classicAgentFromSummary(region string, a batypes.AgentSummary) *sa.Finding {
	id := str(a.AgentId)
	if id == "" {
		return nil
	}
	if _, seen := s.fs.Lookup(models.ShadowKindPlatformAgent, classicAgentExternalID(region, id)); seen {
		return nil
	}
	f := s.fs.Get(models.ShadowKindPlatformAgent, classicAgentExternalID(region, id))
	f.DisplayName = sa.SanitizeName(str(a.AgentName))
	f.Scope = region
	f.State = string(a.AgentStatus)
	f.Attributes["region"] = region
	f.Attributes["status"] = string(a.AgentStatus)
	if v := str(a.LatestAgentVersion); v != "" {
		f.Attributes["agentVersion"] = v
	}
	f.AddSignal(sa.Signal{Code: sa.SigBedrockClassicAgent, Source: callListAgents, Observed: map[string]any{"agentStatus": string(a.AgentStatus)}})
	return f
}

// getClassicAgent adds the model, orchestration, role and creation time.
// instruction and promptOverrideConfiguration (@sensitive prompts) and the
// description are never read into the finding.
func (s *scan) getClassicAgent(ctx context.Context, rc *regionClients, id string, f *sa.Finding) {
	out, err := rc.agents.GetAgent(ctx, &ba.GetAgentInput{AgentId: aws.String(id)})
	if err != nil {
		s.calls.Record(callGetAgent, 0, awsError(err))
		return
	}
	s.calls.Record(callGetAgent, 1, nil)
	if out.Agent == nil || f == nil {
		return
	}
	a := out.Agent
	f.Attributes["resourceId"] = str(a.AgentArn)
	if m := str(a.FoundationModel); m != "" {
		f.Attributes["model"] = m
	}
	if a.OrchestrationType != "" {
		f.Attributes["orchestrationType"] = string(a.OrchestrationType)
	}
	if role := str(a.AgentResourceRoleArn); role != "" {
		f.Attributes["roleArn"] = role
	}
	f.ProviderCreatedAt = timePtr(a.CreatedAt)
}

// actionGroups records the names of the agent's action groups as tools.
func (s *scan) actionGroups(ctx context.Context, rc *regionClients, id string, f *sa.Finding) {
	p := ba.NewListAgentActionGroupsPaginator(rc.agents, &ba.ListAgentActionGroupsInput{
		AgentId: aws.String(id), AgentVersion: aws.String(draftVersion), MaxResults: aws.Int32(pageSize), NextToken: nil,
	})
	var tools []sa.ToolRef
	n, err := paginate(ctx, maxPages, p.HasMorePages, func(ctx context.Context) (int, error) {
		out, err := p.NextPage(ctx)
		if err != nil {
			return 0, err
		}
		for _, g := range out.ActionGroupSummaries {
			if name := str(g.ActionGroupName); name != "" {
				tools = append(tools, sa.ToolRef{Type: "action_group", Name: name})
			}
		}
		return len(out.ActionGroupSummaries), nil
	})
	s.calls.Record(callListActions, n, err)
	if f != nil && len(tools) > 0 {
		f.Attributes["tools"] = tools
		f.Attributes["toolCount"] = len(tools)
	}
}
