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

package gwctl

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo/gwrepomocks"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
)

type servingGuard struct{}

func (servingGuard) ServesEnvironment(context.Context, *gorm.DB, uuid.UUID, uuid.UUID) (bool, error) {
	return true, nil
}

func TestRemoveGatewayFromEnvironment_ServedEnvironmentIsAConflict(t *testing.T) {
	gateway := &gwmodels.Gateway{UUID: uuid.New()}
	repo := &gwrepomocks.GatewayRepositoryMock{
		GetByUUIDFunc:                func(string) (*gwmodels.Gateway, error) { return gateway, nil },
		TransactionFunc:              func(fn func(tx *gorm.DB) error) error { return fn(nil) },
		DeleteEnvironmentMappingFunc: func(string, string) error { return nil },
	}
	repo.WithTxFunc = func(*gorm.DB) gwrepo.GatewayRepository { return repo }
	ocClient := &gwtest.Core{
		ListEnvironmentsFunc: func(context.Context, string) ([]*modkit.Environment, error) {
			return []*modkit.Environment{{ID: defaultEnvUUID, Name: "default"}}, nil
		},
	}
	ctrl := &gatewayController{
		gatewayService: gwsvc.NewPlatformGatewayService(repo).WithEnvironmentGuard(servingGuard{}),
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

func TestRemoveGatewayFromEnvironment_SelfHostedIsAConflict(t *testing.T) {
	gateway := &gwmodels.Gateway{UUID: uuid.New(), Hosting: gwmodels.GatewayHostingSelfHosted}
	repo := &gwrepomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(string) (*gwmodels.Gateway, error) { return gateway, nil },
		// TransactionFunc left nil: the refusal must come before any mutation.
	}
	ocClient := &gwtest.Core{
		ListEnvironmentsFunc: func(context.Context, string) ([]*modkit.Environment, error) {
			return []*modkit.Environment{{ID: defaultEnvUUID, Name: "default"}}, nil
		},
	}
	ctrl := &gatewayController{
		gatewayService: gwsvc.NewPlatformGatewayService(repo),
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
	if !strings.Contains(w.Body.String(), gwerr.ErrCodeGatewayEnvironmentFixed) {
		t.Errorf("body = %s, want reason code %s", w.Body.String(), gwerr.ErrCodeGatewayEnvironmentFixed)
	}
}
