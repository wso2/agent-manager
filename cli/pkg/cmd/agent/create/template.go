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

package create

// agentTemplate is the manifest scaffold emitted by `amctl agent create
// --template`. It is a static document (not a marshalled struct) so it can
// carry comments documenting the variants; TestAgentTemplate_RoundTrip keeps
// it in lockstep with the generated request types.
const agentTemplate = `# Agent manifest for: amctl agent create -f <file>
# Replace <placeholders>. Commented blocks document optional fields and variants.
apiVersion: agent-manager.wso2.com/v1alpha1
kind: Agent
spec:
  name: <agent-name>            # unique resource name
  displayName: <Display Name>
  # description: <optional description>
  agentType:
    type: agent-api             # external provisioning: external-agent-api
    subType: chat-api           # or: custom-api, a2a-agent (see inputInterface below)
  provisioning:
    type: internal              # or: external (then omit repository, build,
                                #     inputInterface and configurations)
    repository:
      url: <https://github.com/org/repo>
      branch: <main>
      appPath: /<path-within-repo>    # must start with /
      # secretRef: <git-secret-name>    # private repositories
    # --- prebuilt Agent Kind variant: replace repository with ---
    # agentKind:
    #   name: <agent-kind-name>
    #   version: <version>
  build:
    type: buildpack             # or: docker
    buildpack:
      language: <python>
      languageVersion: "<3.11>"         # required (only ballerina needs none)
      runCommand: <python main.py>      # required (only ballerina needs none)
    # --- docker variant: replace buildpack with ---
    # docker:
    #   dockerfilePath: </Dockerfile>
  inputInterface:
    type: HTTP
    # --- custom-api only (all three required) ---
    # port: 8000
    # basePath: </api>
    # schema:
    #   path: /<openapi.yaml>          # must start with /
    # --- a2a-agent only ---
    # An A2A agent needs only a port: it serves its own agent card, so there is
    # no OpenAPI document to point at.
    # port: 9099
  # configurations:
  #   env:
  #     - key: <KEY>
  #       value: <value>
  #     - key: <SECRET_KEY>             # sensitive value, stored as a secret
  #       value: <secret-value>
  #       isSensitive: true
  #     - key: <FROM_SECRET>            # reference an existing secret
  #       secretRef: <secret-name>
  #   enableAutoInstrumentation: false
  # modelConfig:                        # attach a configured LLM provider
  #   - providerName: <provider-handle>
  #     environmentVariables:
  #       - key: url
  #         name: <ENV_VAR_FOR_URL>
  #       - key: apikey
  #         name: <ENV_VAR_FOR_API_KEY>
`
