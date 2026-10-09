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
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

func TestMaskWebhookURL(t *testing.T) {
	assert.Equal(t, "https://hooks.slack.com/…", maskWebhookURL("https://hooks.slack.com/services/T0/B0/secret"))
	assert.Equal(t, "https://h.example.com/…", maskWebhookURL("https://h.example.com?token=abc"))
	assert.Equal(t, "https://h.example.com", maskWebhookURL("https://h.example.com/"))
	assert.Equal(t, "", maskWebhookURL("not a url"))
}

func TestToWebhookResponse_MasksURLForReaders(t *testing.T) {
	e := &models.WebhookEndpoint{URL: "https://hooks.example.com/t/secret-token", Scope: models.WebhookScopeAgent}
	assert.Equal(t, "https://hooks.example.com/…", toWebhookResponse(e, "", false).Url)
	assert.Equal(t, e.URL, toWebhookResponse(e, "", true).Url)
}
