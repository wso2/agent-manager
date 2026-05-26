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

package controllers

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/instrumentation"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// AgentBuildOptionsController serves the create-agent form's option data
// (Python versions + instrumentation catalog) over HTTP.
type AgentBuildOptionsController interface {
	GetAgentBuildOptions(w http.ResponseWriter, r *http.Request)
}

type agentBuildOptionsController struct {
	catalog                 *instrumentation.Catalog
	supportedPythonVersions []string
	defaultPythonVersion    string
}

// NewAgentBuildOptionsController constructs the controller.
// supportedPythonVersions is the platform's bare-minor Python list,
// typically utils.SupportedPythonVersions(). defaultPythonVersion is the
// form's pre-selected Python; it must appear in supportedPythonVersions.
func NewAgentBuildOptionsController(
	catalog *instrumentation.Catalog,
	supportedPythonVersions []string,
	defaultPythonVersion string,
) AgentBuildOptionsController {
	return &agentBuildOptionsController{
		catalog:                 catalog,
		supportedPythonVersions: supportedPythonVersions,
		defaultPythonVersion:    defaultPythonVersion,
	}
}

// GetAgentBuildOptions handles GET /orgs/{orgName}/agent-build-options.
// Returns the Python set the platform supports and the effective
// instrumentation catalog. Org-scoped for routing/auth consistency with
// catalog_routes.go; the response is identical across orgs.
func (c *agentBuildOptionsController) GetAgentBuildOptions(w http.ResponseWriter, _ *http.Request) {
	versions := c.catalog.All()
	// Newest-first by numeric-component compare. Lex sort inverts
	// "0.10.0" vs "0.2.1" once the catalog grows past 0.9.x.
	sorted := make([]instrumentation.Version, len(versions))
	copy(sorted, versions)
	sort.Slice(sorted, func(i, j int) bool {
		return compareVersionsDesc(sorted[i].Version, sorted[j].Version)
	})

	entries := make([]spec.AgentBuildOptionsInstrumentationEntry, 0, len(sorted))
	for _, v := range sorted {
		entries = append(entries, spec.AgentBuildOptionsInstrumentationEntry{
			Version:        v.Version,
			PythonVersions: v.PythonVersions,
		})
	}

	resp := spec.AgentBuildOptionsResponse{
		Python: spec.AgentBuildOptionsPython{
			DefaultVersion:    c.defaultPythonVersion,
			SupportedVersions: c.supportedPythonVersions,
		},
		Instrumentation: spec.AgentBuildOptionsInstrumentation{
			DefaultVersion: c.catalog.Default(),
			Versions:       entries,
		},
	}

	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

// compareVersionsDesc reports whether a should sort before b in
// newest-first order. Compares dot-separated components as integers;
// non-numeric components fall back to lexical compare on that component
// so the function stays total on any pair of strings.
func compareVersionsDesc(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		var ai, bi string
		if i < len(pa) {
			ai = pa[i]
		}
		if i < len(pb) {
			bi = pb[i]
		}
		an, aErr := strconv.Atoi(ai)
		bn, bErr := strconv.Atoi(bi)
		if aErr == nil && bErr == nil {
			if an != bn {
				return an > bn
			}
			continue
		}
		if ai != bi {
			return ai > bi
		}
	}
	return false
}
