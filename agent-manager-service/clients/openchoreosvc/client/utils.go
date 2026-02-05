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
//

package client

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/gen"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/config"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
)

// extractEndpointURLsFromRelease extracts endpoint URLs from HTTPRoute resources in the release
func extractEndpointURLsFromRelease(release *gen.ReleaseResponse) ([]models.Endpoint, error) {
	var endpoints []models.Endpoint

	if release == nil || release.Spec.Resources == nil {
		return endpoints, nil
	}

	for _, resource := range *release.Spec.Resources {
		obj := resource.Object
		if len(obj) == 0 {
			continue
		}

		kind, _ := obj["kind"].(string)
		if kind != ResourceKindHTTPRoute {
			continue
		}

		hostnames, found, err := unstructured.NestedStringSlice(obj, "spec", "hostnames")
		if err != nil {
			return nil, fmt.Errorf("error extracting hostnames from HTTPRoute: %w", err)
		}
		if !found || len(hostnames) == 0 {
			return nil, fmt.Errorf("HTTPRoute missing hostnames")
		}
		hostname := hostnames[0]

		pathValue, err := extractPathValue(obj)
		if err != nil {
			return nil, fmt.Errorf("error extracting path from HTTPRoute: %w", err)
		}

		port := config.GetConfig().DefaultGatewayPort
		url := fmt.Sprintf("http://%s:%d", hostname, port)
		if pathValue != "" {
			url = fmt.Sprintf("http://%s:%d%s", hostname, port, pathValue)
		}

		endpoints = append(endpoints, models.Endpoint{
			URL:        url,
			Visibility: EndpointVisibilityPublic,
		})
	}

	return endpoints, nil
}

// extractPathValue extracts the path value from an HTTPRoute object
func extractPathValue(obj map[string]interface{}) (string, error) {
	rules, found, err := unstructured.NestedSlice(obj, "spec", "rules")
	if err != nil {
		return "", fmt.Errorf("error extracting rules from HTTPRoute: %w", err)
	}
	// Rules are optional - empty rules means match-all behavior
	if !found || len(rules) == 0 {
		return "", nil
	}

	rule0, ok := rules[0].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid rule format in HTTPRoute")
	}

	matches, found, err := unstructured.NestedSlice(rule0, "matches")
	if err != nil {
		return "", fmt.Errorf("error extracting matches from HTTPRoute: %w", err)
	}
	// Matches are optional - empty matches means match-all behavior
	if !found || len(matches) == 0 {
		return "", nil
	}

	match0, ok := matches[0].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid match format in HTTPRoute")
	}

	pathValue, found, err := unstructured.NestedString(match0, "path", "value")
	if err != nil {
		return "", fmt.Errorf("error extracting path value from HTTPRoute: %w", err)
	}
	// Path value is optional - missing path means prefix match on "/"
	if !found {
		return "", nil
	}
	return pathValue, nil
}
