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
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/agentregistrycontrol"
	"github.com/aws/aws-sdk-go-v2/service/bedrockagent"
	"github.com/aws/aws-sdk-go-v2/service/bedrockagentcorecontrol"

	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
)

// regionClients are the SDK clients for one region, each pointed at the
// endpoint the connection configured for it.
type regionClients struct {
	region    string
	agentCore *bedrockagentcorecontrol.Client
	registry  *agentregistrycontrol.Client
	agents    *bedrockagent.Client
}

// newRegionClients builds the clients for region from the connection's
// endpoints: the URL set for the region, else the template filled in. An
// endpoint that cannot be expanded is an error.
// retryer, when set, replaces the SDK's standard retryer (the connector's
// retry policy); nil keeps the one sa.AWSConfig configures.
func newRegionClients(conn sa.Connection, httpClient *http.Client, region string, retryer func() aws.Retryer) (*regionClients, error) {
	ep := func(key string) (string, error) { return sa.RegionEndpoint(conn, key, region) }
	stsEP, err := ep(sa.EPBedrockSTS)
	if err != nil {
		return nil, err
	}
	cfg, err := sa.AWSConfig(httpClient, region, stsEP, sa.AWSCredentials{
		AccessKeyID:     conn.Config.AccessKeyID,
		SecretAccessKey: conn.Secret.SecretAccessKey,
		SessionToken:    conn.Secret.SessionToken,
		RoleARN:         conn.Config.RoleARN,
		ExternalID:      conn.Config.ExternalID,
	})
	if err != nil {
		return nil, err
	}
	if retryer != nil {
		cfg.Retryer = retryer
	}
	endpoints := map[string]string{}
	for _, k := range []string{sa.EPBedrockAgentCore, sa.EPBedrockRegistry, sa.EPBedrockAgent} {
		v, err := ep(k)
		if err != nil {
			return nil, err
		}
		endpoints[k] = v
	}
	return &regionClients{
		region: region,
		agentCore: bedrockagentcorecontrol.NewFromConfig(cfg, func(o *bedrockagentcorecontrol.Options) {
			o.BaseEndpoint = aws.String(endpoints[sa.EPBedrockAgentCore])
		}),
		registry: agentregistrycontrol.NewFromConfig(cfg, func(o *agentregistrycontrol.Options) {
			o.BaseEndpoint = aws.String(endpoints[sa.EPBedrockRegistry])
		}),
		agents: bedrockagent.NewFromConfig(cfg, func(o *bedrockagent.Options) {
			o.BaseEndpoint = aws.String(endpoints[sa.EPBedrockAgent])
		}),
	}, nil
}
