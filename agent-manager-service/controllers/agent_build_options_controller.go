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

	"github.com/wso2/agent-manager/agent-manager-service/instrumentation"
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

type instrumentationVersionDTO struct {
	Version        string   `json:"version"`
	PythonVersions []string `json:"pythonVersions"`
}

type pythonOptionsDTO struct {
	DefaultVersion    string   `json:"defaultVersion"`
	SupportedVersions []string `json:"supportedVersions"`
}

type instrumentationOptionsDTO struct {
	DefaultVersion string                      `json:"defaultVersion"`
	Versions       []instrumentationVersionDTO `json:"versions"`
}

type agentBuildOptionsResponse struct {
	Python          pythonOptionsDTO          `json:"python"`
	Instrumentation instrumentationOptionsDTO `json:"instrumentation"`
}

// GetAgentBuildOptions handles GET /orgs/{orgName}/agent-build-options.
// Returns the Python set the platform supports and the effective
// instrumentation catalog. Org-scoped for routing/auth consistency with
// catalog_routes.go; the response is identical across orgs.
func (c *agentBuildOptionsController) GetAgentBuildOptions(w http.ResponseWriter, _ *http.Request) {
	versions := c.catalog.All()
	// Newest-first by version string. Lexicographic is sufficient while
	// the catalog stays within a single semver-major; revisit if mixed
	// majors ever land.
	sorted := make([]instrumentation.Version, len(versions))
	copy(sorted, versions)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Version > sorted[j].Version })

	resp := agentBuildOptionsResponse{
		Python: pythonOptionsDTO{
			DefaultVersion:    c.defaultPythonVersion,
			SupportedVersions: c.supportedPythonVersions,
		},
		Instrumentation: instrumentationOptionsDTO{
			DefaultVersion: c.catalog.Default(),
			Versions:       make([]instrumentationVersionDTO, 0, len(sorted)),
		},
	}
	for _, v := range sorted {
		resp.Instrumentation.Versions = append(resp.Instrumentation.Versions, instrumentationVersionDTO{
			Version:        v.Version,
			PythonVersions: v.PythonVersions,
		})
	}

	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}
