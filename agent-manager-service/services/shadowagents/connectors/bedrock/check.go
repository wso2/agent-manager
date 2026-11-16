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
	reg "github.com/aws/aws-sdk-go-v2/service/agentregistrycontrol"
	ba "github.com/aws/aws-sdk-go-v2/service/bedrockagent"
	ac "github.com/aws/aws-sdk-go-v2/service/bedrockagentcorecontrol"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
)

// Test implements sa.Connector: one cheap read per selected service, in the
// first configured region. OK means the credential works and every
// selected service can be listed.
func (c *Connector) Test(ctx context.Context, conn sa.Connection) models.ShadowConnectionTestInfo {
	regions := cleanRegions(conn.Config.Regions)
	if len(regions) == 0 {
		return models.ShadowConnectionTestInfo{OK: false, Checks: []models.ShadowConnectionCheck{{
			Name: callConfigure, Outcome: models.ShadowCallFailed, Message: "at least one region is required", Unlocks: nil,
		}}}
	}
	rc, err := newRegionClients(conn, c.http, regions[0], c.retryer)
	if err != nil {
		return models.ShadowConnectionTestInfo{OK: false, Checks: []models.ShadowConnectionCheck{{
			Name: callConfigure, Outcome: models.ShadowCallFailed, Message: sa.ScrubText(err.Error()), Unlocks: nil,
		}}}
	}
	probes := []struct {
		name    string
		option  string
		unlocks []string
		call    func(context.Context) error
	}{
		{callListRuntimes, sa.OptBedrockAgentCore, []string{sa.SigBedrockAgentCoreRuntime}, func(ctx context.Context) error {
			_, err := rc.agentCore.ListAgentRuntimes(ctx, &ac.ListAgentRuntimesInput{MaxResults: aws.Int32(1), NextToken: nil})
			return err
		}},
		{callListRegistries, sa.OptBedrockRegistry, []string{sa.SigBedrockRegistryAgent}, func(ctx context.Context) error {
			_, err := rc.registry.ListRegistries(ctx, &reg.ListRegistriesInput{Filters: nil, MaxResults: aws.Int32(1), NextToken: nil})
			return err
		}},
		{callListAgents, sa.OptBedrockClassicAgents, []string{sa.SigBedrockClassicAgent}, func(ctx context.Context) error {
			_, err := rc.agents.ListAgents(ctx, &ba.ListAgentsInput{MaxResults: aws.Int32(1), NextToken: nil})
			return err
		}},
	}
	info := models.ShadowConnectionTestInfo{OK: false, Checks: nil}
	ran, failed := 0, 0
	for _, p := range probes {
		if !sa.ServiceEnabled(conn.Config, p.option) {
			info.Checks = append(info.Checks, models.ShadowConnectionCheck{
				Name: p.name, Outcome: models.ShadowCallSkipped, Message: sa.ReasonServiceOff, Unlocks: p.unlocks,
			})
			continue
		}
		err := awsError(p.call(ctx))
		info.Checks = append(info.Checks, models.ShadowConnectionCheck{
			Name: p.name, Outcome: sa.OutcomeOf(err), Message: sa.MessageOf(err), Unlocks: p.unlocks,
		})
		ran++
		if err != nil {
			failed++
			if ran == 1 && isAuthFailure(err) {
				// The credential was refused: the other calls would be too.
				break
			}
		}
	}
	info.OK = ran > 0 && failed == 0
	return info
}
