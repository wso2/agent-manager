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
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/services"
)

type servingGuard struct{}

func (servingGuard) ServesEnvironment(context.Context, *gorm.DB, uuid.UUID, uuid.UUID) (bool, error) {
	return true, nil
}

func TestRemoveGatewayFromEnvironment_ServedEnvironmentIsAConflict(t *testing.T) {
	gateway := &models.Gateway{UUID: uuid.New()}
	repo := &repomocks.GatewayRepositoryMock{
		GetByUUIDFunc:                func(string) (*models.Gateway, error) { return gateway, nil },
		TransactionFunc:              func(fn func(tx *gorm.DB) error) error { return fn(nil) },
		DeleteEnvironmentMappingFunc: func(string, string) error { return nil },
	}
	repo.WithTxFunc = func(*gorm.DB) repositories.GatewayRepository { return repo }
	ocClient := &clientmocks.OpenChoreoClientMock{
		ListEnvironmentsFunc: func(context.Context, string) ([]*models.EnvironmentResponse, error) {
			return []*models.EnvironmentResponse{{UUID: defaultEnvUUID, Name: "default"}}, nil
		},
	}
	ctrl := &gatewayController{
		gatewayService: services.NewPlatformGatewayService(repo, nil).WithEnvironmentGuard(servingGuard{}),
		ocClient:       ocClient,
	}
	req := httptest.NewRequest(http.MethodDelete, "/orgs/acme/gateways/"+gateway.UUID.String()+"/environments/default", nil)
	req.SetPathValue("gatewayID", gateway.UUID.String())
	req.SetPathValue("envID", "default")
	w := httptest.NewRecorder()

	ctrl.RemoveGatewayFromEnvironment(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "undeploy") {
		t.Errorf("body = %s, want it to say to undeploy first", w.Body.String())
	}
}
