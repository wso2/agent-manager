// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

package observabilitysvc

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/clients/requests"
	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/config"
	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/models"
)

// Build log constants
const (
	BuildLogLevelInfo = "INFO"
	BuildLogTypeBuild = "BUILD"
)

type ObservabilitySvcClient interface {
	GetBuildLogs(ctx context.Context, orgName string, projName string, agentName string, buildName string, buildUuid string) (*models.BuildLogsResponse, error)
}

type observabilitySvcClient struct {
	httpClient requests.HttpClient
}

func NewObservabilitySvcClient() ObservabilitySvcClient {
	httpClient := &http.Client{
		Timeout: time.Second * 15,
	}
	return &observabilitySvcClient{
		httpClient: httpClient,
	}
}

// GetBuildLogs retrieves build logs for a specific agent build from the observer service
func (o *observabilitySvcClient) GetBuildLogs(ctx context.Context, orgName string, projName string, agentName string, buildName string, buildUuid string) (*models.BuildLogsResponse, error) {
	// temporary use config to get observer URL since the observer url in dataplane is cluster svc name which is not accessible outside the cluster,
	// so we need to portforward the observer svc and use localhost:port to access the observer service
	baseURL := config.GetConfig().Observer.URL
	logsURL := fmt.Sprintf("%s/api/logs/component/%s", baseURL, agentName)

	requestBody := map[string]interface{}{
		"buildId":   buildName,
		"buildUuid": buildUuid,
		"logLevels": []string{BuildLogLevelInfo},
		"logType":   BuildLogTypeBuild,
	}

	req := &requests.HttpRequest{
		Name:   "observabilitysvc.GetBuildLogs",
		URL:    logsURL,
		Method: http.MethodPost,
	}
	req.SetHeader("Accept", "application/json")
	req.SetJson(requestBody)

	var logsResponse models.BuildLogsResponse
	if err := requests.SendRequest(ctx, o.httpClient, req).ScanResponse(&logsResponse, http.StatusOK); err != nil {
		return nil, fmt.Errorf("observabilitysvc.GetBuildLogs: %w", err)
	}

	return &logsResponse, nil
}
