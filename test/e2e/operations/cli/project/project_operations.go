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

// Package cliproject holds amctl project commands as thin, assertion-backed
// operations over the amctl harness, mirroring operations/project (HTTP).
package cliproject

import (
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	"github.com/wso2/agent-manager/test/e2e/framework/amctl"
)

// Project is the Project document `amctl project create|get --json` prints.
type Project = framework.ProjectDocument

// ProjectList is the list envelope `amctl project list --json` prints.
type ProjectList = framework.ProjectListResponse

// DeleteResult is the data shape of `amctl project delete --json`.
type DeleteResult struct {
	Name    string `json:"name"`
	Deleted bool   `json:"deleted"`
}

// CreateProject runs `amctl project create` and returns the created project.
func CreateProject(g Gomega, h *amctl.Harness, org, name, displayName, description string) Project {
	args := []string{"project", "create", name, "--org", org, "--display-name", displayName, "--json"}
	if description != "" {
		args = append(args, "--description", description)
	}
	return amctl.DecodeData[Project](g, h.Run(args...))
}

// DeleteProject runs `amctl project delete --yes` and returns the delete result.
func DeleteProject(g Gomega, h *amctl.Harness, org, name string) DeleteResult {
	return amctl.DecodeData[DeleteResult](g, h.Run("project", "delete", name, "--org", org, "--yes", "--json"))
}

// GetProject runs `amctl project get` and returns the project.
func GetProject(g Gomega, h *amctl.Harness, org, name string) Project {
	return amctl.DecodeData[Project](g, h.Run("project", "get", name, "--org", org, "--json"))
}

// GetProjectExpectError runs `amctl project get` expecting a non-zero exit
// (e.g. the project no longer exists) and returns the error envelope.
func GetProjectExpectError(g Gomega, h *amctl.Harness, org, name string) amctl.EnvelopeError {
	return h.Run("project", "get", name, "--org", org, "--json").ExpectError(g)
}

// ListProjects runs `amctl project list` and returns the project list.
func ListProjects(g Gomega, h *amctl.Harness, org string) ProjectList {
	return amctl.DecodeData[ProjectList](g, h.Run("project", "list", "--org", org, "--json"))
}

// ProjectNames returns the project names in the list, for membership assertions.
func ProjectNames(l ProjectList) []string {
	names := make([]string, 0, len(l.List))
	for _, p := range l.List {
		names = append(names, p.Metadata.Name)
	}
	return names
}
