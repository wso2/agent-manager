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

package google

import (
	"context"
	"net/url"
	"strconv"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
)

// listEnginesResponse is GoogleCloudAiplatformV1ListReasoningEnginesResponse.
type listEnginesResponse struct {
	ReasoningEngines []reasoningEngine `json:"reasoningEngines"`
	NextPageToken    string            `json:"nextPageToken"`
}

// reasoningEngine holds the fields of GoogleCloudAiplatformV1ReasoningEngine
// the connector keeps. spec.deploymentSpec (env values, secretEnv),
// packageSpec and the source archive are deliberately not declared, so
// they are never decoded.
type reasoningEngine struct {
	Name        string            `json:"name"`
	DisplayName string            `json:"displayName"`
	CreateTime  string            `json:"createTime"`
	Labels      map[string]string `json:"labels"`
	Spec        *engineSpec       `json:"spec"`
}

type engineSpec struct {
	AgentFramework    string          `json:"agentFramework"`
	ServiceAccount    string          `json:"serviceAccount"`
	EffectiveIdentity string          `json:"effectiveIdentity"`
	SourceCodeSpec    *sourceCodeSpec `json:"sourceCodeSpec"`
}

type sourceCodeSpec struct {
	GitSource *struct {
		URL string `json:"url"`
	} `json:"gitSource"`
}

// listEngines lists the Agent Runtime engines of one project and location.
func (s *scanner) listEngines(ctx context.Context, project, location string) {
	if !s.proceed(ctx, callEngines) {
		return
	}
	base, err := runtimeBase(s.conn, location)
	if err != nil {
		s.record(callEngines, 0, &sa.CallError{Outcome: models.ShadowCallFailed, Message: err.Error()})
		return
	}
	u := base + "/v1/projects/" + url.PathEscape(project) + "/locations/" + url.PathEscape(location) + "/reasoningEngines"
	s.paginate(ctx, callEngines, maxEnginePages, func(token string) (string, int, error) {
		q := url.Values{"pageSize": {strconv.Itoa(enginesPageSize)}}
		if token != "" {
			q.Set("pageToken", token)
		}
		var resp listEnginesResponse
		if err := s.api.GetJSON(ctx, u, q, &resp); err != nil {
			return "", 0, err
		}
		for i := range resp.ReasoningEngines {
			s.addEngine(project, location, &resp.ReasoningEngines[i])
		}
		return resp.NextPageToken, len(resp.ReasoningEngines), nil
	})
}

// addEngine turns an engine into a runtime finding (ExternalID = the
// engine's resource name) and indexes it for the later calls.
func (s *scanner) addEngine(project, location string, re *reasoningEngine) {
	if re.Name == "" {
		return
	}
	ref, ok := parseEngineRef(re.Name)
	if !ok {
		ref = engineRef{project: project, location: location, id: lastSegment(re.Name)}
	}
	f := s.findings.Get(models.ShadowKindRuntime, re.Name)
	f.DisplayName = sa.SanitizeName(re.DisplayName)
	if f.DisplayName == "" {
		f.DisplayName = ref.id
	}
	f.Scope = scopeOf(project, ref.location)
	f.Attributes["project"] = project
	f.Attributes["location"] = ref.location
	if len(re.Labels) > 0 {
		f.Attributes["labels"] = re.Labels
	}
	if spec := re.Spec; spec != nil {
		if spec.AgentFramework != "" {
			f.Attributes["framework"] = spec.AgentFramework
		}
		switch {
		case spec.ServiceAccount != "":
			f.Attributes["serviceAccount"] = spec.ServiceAccount
		case spec.EffectiveIdentity != "":
			f.Attributes["serviceAccount"] = spec.EffectiveIdentity
		}
		if spec.SourceCodeSpec != nil && spec.SourceCodeSpec.GitSource != nil {
			if repo := sa.RepositoryURL(spec.SourceCodeSpec.GitSource.URL); repo != "" {
				f.Attributes["repositoryUrl"] = repo
			}
		}
	}
	f.ProviderCreatedAt = parseTime(re.CreateTime)
	_, hasFramework := f.Attributes["framework"]
	_, hasRepo := f.Attributes["repositoryUrl"]
	f.AddSignal(sa.Signal{Code: sa.SigGoogleAgentEngine, Source: callEngines, Observed: map[string]any{
		"frameworkSet": hasFramework, "gitSource": hasRepo,
	}})

	s.addAlias(project, ref.project)
	for _, e := range s.engines {
		if e.finding == f {
			return
		}
	}
	s.engines = append(s.engines, &engine{finding: f, project: project, nameProject: ref.project, location: ref.location, id: ref.id})
}

func lastSegment(name string) string {
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '/' {
			return name[i+1:]
		}
	}
	return name
}
