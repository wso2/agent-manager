//
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

package client

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/gen"
)

func TestIsSystemLabelKey(t *testing.T) {
	assert.True(t, isSystemLabelKey("openchoreo.dev/organization"))
	assert.True(t, isSystemLabelKey("openchoreo.dev/"))
	assert.False(t, isSystemLabelKey("team"))
	assert.False(t, isSystemLabelKey(""))
	assert.False(t, isSystemLabelKey("foo/bar"))
}

func TestExtractUserLabels(t *testing.T) {
	t.Run("nil labels", func(t *testing.T) {
		assert.Nil(t, extractUserLabels(nil))
	})

	t.Run("empty map", func(t *testing.T) {
		empty := map[string]string{}
		assert.Nil(t, extractUserLabels(&empty))
	})

	t.Run("all system labels", func(t *testing.T) {
		labels := map[string]string{
			string(LabelKeyOrganizationName): "acme",
			string(LabelKeyProjectName):      "proj",
		}
		assert.Nil(t, extractUserLabels(&labels))
	})

	t.Run("mixed system and user labels", func(t *testing.T) {
		labels := map[string]string{
			string(LabelKeyOrganizationName): "acme",
			"env":                            "prod",
			"team":                           "ml",
		}
		got := extractUserLabels(&labels)
		assert.Equal(t, map[string]string{"env": "prod", "team": "ml"}, got)
	})

	t.Run("all user labels", func(t *testing.T) {
		labels := map[string]string{"env": "prod", "team": "ml"}
		got := extractUserLabels(&labels)
		assert.Equal(t, labels, got)
	})
}

func TestMergeUserLabels(t *testing.T) {
	t.Run("nil existing", func(t *testing.T) {
		got := mergeUserLabels(nil, map[string]string{"env": "prod"})
		assert.Equal(t, map[string]string{"env": "prod"}, got)
	})

	t.Run("preserves system labels, replaces user labels wholesale", func(t *testing.T) {
		existing := map[string]string{
			string(LabelKeyOrganizationName): "acme",
			string(LabelKeyProjectName):      "proj",
			"env":                            "dev", // old user label, dropped
			"team":                           "old", // old user label, will be overwritten
		}
		got := mergeUserLabels(&existing, map[string]string{"team": "ml", "tier": "critical"})
		assert.Equal(t, map[string]string{
			string(LabelKeyOrganizationName): "acme",
			string(LabelKeyProjectName):      "proj",
			"team":                           "ml",
			"tier":                           "critical",
		}, got)
	})

	t.Run("empty new user labels clears all user labels, keeps system", func(t *testing.T) {
		existing := map[string]string{
			string(LabelKeyOrganizationName): "acme",
			"env":                            "prod",
		}
		got := mergeUserLabels(&existing, map[string]string{})
		assert.Equal(t, map[string]string{string(LabelKeyOrganizationName): "acme"}, got)
	})
}

func TestAddUserLabels(t *testing.T) {
	t.Run("merges user labels into the target map", func(t *testing.T) {
		labels := map[string]string{string(LabelKeyOrganizationName): "acme"}
		addUserLabels(labels, map[string]string{"env": "prod"})
		assert.Equal(t, map[string]string{
			string(LabelKeyOrganizationName): "acme",
			"env":                            "prod",
		}, labels)
	})

	t.Run("nil user labels is a no-op", func(t *testing.T) {
		labels := map[string]string{string(LabelKeyOrganizationName): "acme"}
		addUserLabels(labels, nil)
		assert.Equal(t, map[string]string{string(LabelKeyOrganizationName): "acme"}, labels)
	})
}

func TestWithResourceLabels(t *testing.T) {
	t.Run("none configured leaves labels untouched", func(t *testing.T) {
		c := &openChoreoClient{}
		assert.Nil(t, c.withResourceLabels(nil))
		labels := map[string]string{"team": "ml"}
		assert.Equal(t, map[string]string{"team": "ml"}, *c.withResourceLabels(&labels))
	})

	c := &openChoreoClient{resourceLabels: map[string]string{"example.com/product": "agent-manager"}}

	t.Run("nil pointer allocates the map", func(t *testing.T) {
		got := c.withResourceLabels(nil)
		require.NotNil(t, got)
		assert.Equal(t, map[string]string{"example.com/product": "agent-manager"}, *got)
	})

	t.Run("nil map behind the pointer allocates the map", func(t *testing.T) {
		var labels map[string]string
		assert.Equal(t, map[string]string{"example.com/product": "agent-manager"}, *c.withResourceLabels(&labels))
	})

	t.Run("keeps existing labels and overwrites a stale value", func(t *testing.T) {
		labels := map[string]string{
			string(LabelKeyComponentName): "agent-1",
			"example.com/product":         "stale",
		}
		assert.Equal(t, map[string]string{
			string(LabelKeyComponentName): "agent-1",
			"example.com/product":         "agent-manager",
		}, *c.withResourceLabels(&labels))
	})
}

func TestValidateResourceLabels(t *testing.T) {
	require.NoError(t, validateResourceLabels(nil))
	require.NoError(t, validateResourceLabels(map[string]string{"cloud.wso2.com/product-name": "agent-manager"}))
	require.Error(t, validateResourceLabels(map[string]string{"bad key!": "v"}))
	require.Error(t, validateResourceLabels(map[string]string{"example.com/product": "not a valid value"}))
}

func TestBuildInternalAgentFromKindComponentRequestBody_UserLabels(t *testing.T) {
	req := CreateComponentRequest{
		Name:        "agent-1",
		DisplayName: "Agent 1",
		AgentType:   AgentTypeConfig{Type: "agent-api", SubType: "chat-api"},
		AgentKind:   &AgentKindRef{Name: "kind-1", Version: "v1"},
		Build:       &BuildConfig{Type: "buildpack", Buildpack: &BuildpackConfig{Language: "python"}},
		Labels:      map[string]string{"team": "ml"},
	}
	body, err := buildInternalAgentFromKindComponentRequestBody("ns", "proj", req)
	require.NoError(t, err)
	require.NotNil(t, body.Metadata.Labels)
	labels := *body.Metadata.Labels
	assert.Equal(t, "ml", labels["team"])
	assert.Equal(t, "kind-1", labels[string(LabelKeyAgentKindName)])
	// System keys plus the one user key, no collision.
	assert.Len(t, labels, 7)
}

func TestBuildExternalAgentComponentRequestBody_UserLabels(t *testing.T) {
	req := CreateComponentRequest{
		Name:             "agent-1",
		DisplayName:      "Agent 1",
		ProvisioningType: "external",
		AgentType:        AgentTypeConfig{Type: "external-agent-api", SubType: "custom-api"},
		Labels:           map[string]string{"team": "ml"},
	}
	body, err := buildExternalAgentComponentRequestBody("ns", "proj", req)
	require.NoError(t, err)
	require.NotNil(t, body.Metadata.Labels)
	labels := *body.Metadata.Labels
	assert.Equal(t, "ml", labels["team"])
	assert.Equal(t, "external", labels[string(LabelKeyProvisioningType)])
	assert.Len(t, labels, 2)
}

func TestBuildInternalAgentFromSourceComponentRequestBody_UserLabels(t *testing.T) {
	req := CreateComponentRequest{
		Name:             "agent-1",
		DisplayName:      "Agent 1",
		ProvisioningType: "internal",
		AgentType:        AgentTypeConfig{Type: "agent-api", SubType: "chat-api"},
		Build:            &BuildConfig{Type: "buildpack", Buildpack: &BuildpackConfig{Language: "python", LanguageVersion: "3.11"}},
		InputInterface:   &InputInterfaceConfig{BasePath: "/"},
		Labels:           map[string]string{"team": "ml"},
	}
	body, err := buildInternalAgentFromSourceComponentRequestBody("ns", "proj", req)
	require.NoError(t, err)
	require.NotNil(t, body.Metadata.Labels)
	labels := *body.Metadata.Labels
	assert.Equal(t, "ml", labels["team"])
	assert.Equal(t, "python", labels[string(LabelKeyAgentLanguage)])
	// provisioning-type, agent-sub-type, build-source, agent-language, agent-language-version, team
	assert.Len(t, labels, 6)
}

func TestConvertComponentFromTyped_Labels(t *testing.T) {
	labels := map[string]string{
		string(LabelKeyOrganizationName): "acme",
		string(LabelKeyProvisioningType): "internal",
		"env":                            "prod",
		"team":                           "ml",
	}
	comp := &gen.Component{
		Metadata: gen.ObjectMeta{
			Name:   "agent-1",
			Labels: &labels,
		},
		Spec: &gen.ComponentSpec{
			ComponentType: struct {
				Kind *gen.ComponentSpecComponentTypeKind `json:"kind,omitempty"`
				Name string                              `json:"name"`
			}{Name: "internal-agent/agent-api"},
			Owner: struct {
				ProjectName string `json:"projectName"`
			}{ProjectName: "proj"},
		},
	}

	agent, err := convertComponentFromTyped(comp)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"env": "prod", "team": "ml"}, agent.Labels)
}

func TestBuildInternalAgentFromKindComponentRequestBody_RoutePath(t *testing.T) {
	req := CreateComponentRequest{
		Name:        "agent-1",
		DisplayName: "Agent 1",
		AgentType:   AgentTypeConfig{Type: "agent-api", SubType: "chat-api"},
		AgentKind:   &AgentKindRef{Name: "kind-1", Version: "v1"},
		Build:       &BuildConfig{Type: "buildpack", Buildpack: &BuildpackConfig{Language: "python"}},
	}
	body, err := buildInternalAgentFromKindComponentRequestBody("ns", "proj", req)
	require.NoError(t, err)
	require.NotNil(t, body.Spec)
	require.NotNil(t, body.Spec.Parameters)
	// The chart prefixes the slash, so the parameter carries a bare segment.
	assert.Equal(t, "agent-1", (*body.Spec.Parameters)["routePath"])
}

func TestBuildInternalAgentFromSourceComponentRequestBody_RoutePath(t *testing.T) {
	req := CreateComponentRequest{
		Name:             "agent-1",
		DisplayName:      "Agent 1",
		ProvisioningType: "internal",
		AgentType:        AgentTypeConfig{Type: "agent-api", SubType: "chat-api"},
		Build:            &BuildConfig{Type: "buildpack", Buildpack: &BuildpackConfig{Language: "python", LanguageVersion: "3.11"}},
		InputInterface:   &InputInterfaceConfig{BasePath: "/"},
	}
	body, err := buildInternalAgentFromSourceComponentRequestBody("ns", "proj", req)
	require.NoError(t, err)
	require.NotNil(t, body.Spec)
	require.NotNil(t, body.Spec.Parameters)
	assert.Equal(t, "agent-1", (*body.Spec.Parameters)["routePath"])
}

func TestBuildExternalAgentComponentRequestBody_NoRoutePath(t *testing.T) {
	// External agents render no HTTPRoute at all (external-agent-api.yaml), so a
	// routePath would be a parameter nothing reads.
	req := CreateComponentRequest{
		Name:             "agent-1",
		DisplayName:      "Agent 1",
		ProvisioningType: "external",
		AgentType:        AgentTypeConfig{Type: "external-agent-api", SubType: "custom-api"},
	}
	body, err := buildExternalAgentComponentRequestBody("ns", "proj", req)
	require.NoError(t, err)
	require.NotNil(t, body.Spec)
	assert.Nil(t, body.Spec.Parameters)
}

func TestBuildInternalAgentFromKindComponentRequestBody_KindVersion(t *testing.T) {
	req := CreateComponentRequest{
		Name:        "agent-1",
		DisplayName: "Agent 1",
		AgentType:   AgentTypeConfig{Type: "agent-api", SubType: "chat-api"},
		AgentKind:   &AgentKindRef{Name: "kind-1", Version: "v1.2.0"},
		Build:       &BuildConfig{Type: "buildpack", Buildpack: &BuildpackConfig{Language: "python"}},
	}
	body, err := buildInternalAgentFromKindComponentRequestBody("ns", "proj", req)
	require.NoError(t, err)
	require.NotNil(t, body.Metadata.Labels)
	labels := *body.Metadata.Labels
	assert.Equal(t, "kind-1", labels[string(LabelKeyAgentKindName)])
	assert.Equal(t, "v1.2.0", labels[string(LabelKeyAgentKindVersion)])
}

// A version tag that can't be a label value must fail loudly. Creating the agent
// without the version label would leave an incomplete provenance record that
// nothing downstream could distinguish from a pre-label agent.
func TestBuildInternalAgentFromKindComponentRequestBody_RejectsUnlabelableVersion(t *testing.T) {
	for _, version := range []string{"1.0 (beta)", "v1.0.0+build.1", "-v1", strings.Repeat("v", 64)} {
		t.Run(version, func(t *testing.T) {
			req := CreateComponentRequest{
				Name:        "agent-1",
				DisplayName: "Agent 1",
				AgentType:   AgentTypeConfig{Type: "agent-api", SubType: "chat-api"},
				AgentKind:   &AgentKindRef{Name: "kind-1", Version: version},
				Build:       &BuildConfig{Type: "buildpack", Buildpack: &BuildpackConfig{Language: "python"}},
			}
			_, err := buildInternalAgentFromKindComponentRequestBody("ns", "proj", req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), version)
		})
	}
}

func TestConvertComponentFromTyped_KindVersion(t *testing.T) {
	newComp := func(labels map[string]string) *gen.Component {
		return &gen.Component{
			Metadata: gen.ObjectMeta{
				Name:   "agent-1",
				Labels: &labels,
			},
			Spec: &gen.ComponentSpec{
				ComponentType: struct {
					Kind *gen.ComponentSpecComponentTypeKind `json:"kind,omitempty"`
					Name string                              `json:"name"`
				}{Name: "internal-agent/agent-api"},
				Owner: struct {
					ProjectName string `json:"projectName"`
				}{ProjectName: "proj"},
			},
		}
	}

	t.Run("reads name and version from labels", func(t *testing.T) {
		agent, err := convertComponentFromTyped(newComp(map[string]string{
			string(LabelKeyProvisioningType): "internal",
			string(LabelKeyBuildSource):      BuildSourceKind,
			string(LabelKeyAgentKindName):    "kind-1",
			string(LabelKeyAgentKindVersion): "v1.2.0",
		}))
		require.NoError(t, err)
		assert.Equal(t, "kind-1", agent.KindName)
		assert.Equal(t, "v1.2.0", agent.KindVersion)
	})

	t.Run("agent created before the version label still resolves its kind", func(t *testing.T) {
		agent, err := convertComponentFromTyped(newComp(map[string]string{
			string(LabelKeyProvisioningType): "internal",
			string(LabelKeyBuildSource):      BuildSourceKind,
			string(LabelKeyAgentKindName):    "kind-1",
		}))
		require.NoError(t, err)
		assert.Equal(t, "kind-1", agent.KindName)
		assert.Empty(t, agent.KindVersion)
	})
}
