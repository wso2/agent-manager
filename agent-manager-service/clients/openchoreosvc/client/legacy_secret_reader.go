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
	"context"
	"fmt"
	"net/http"
	"unicode/utf8"

	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
)

// ReadLegacyManagedSecret reads an Agent Manager-owned secret through OpenChoreo's
// legacy secrets API. The generated client decodes Kubernetes' base64 values.
func (c *openChoreoClient) ReadLegacyManagedSecret(ctx context.Context, ouID, secretName string) (map[string]string, error) {
	resp, err := c.ocClient.GetSecretWithResponse(ctx, c.NamespaceFor(ouID), secretName)
	if err != nil {
		return nil, fmt.Errorf("read legacy secret: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("legacy secrets API returned HTTP %d", resp.StatusCode())
	}
	secret := resp.JSON200
	if secret == nil || secret.Metadata.Name != secretName || secret.Metadata.Labels == nil {
		return nil, fmt.Errorf("legacy secrets API returned unexpected metadata")
	}
	labels := *secret.Metadata.Labels
	if labels[secretmanagersvc.LabelKeyManagedBy] != secretmanagersvc.DefaultManagedBy {
		return nil, secretmanagersvc.ErrNotManaged
	}
	if org := labels[string(LabelKeyOrgUUID)]; org != "" && org != ouID {
		return nil, fmt.Errorf("legacy secret belongs to a different organization")
	}
	if secret.Data == nil || len(*secret.Data) == 0 {
		return nil, fmt.Errorf("legacy secret has no values")
	}
	data := make(map[string]string, len(*secret.Data))
	for key, value := range *secret.Data {
		if !utf8.Valid(value) {
			return nil, fmt.Errorf("legacy secret contains a non-UTF-8 value")
		}
		data[key] = string(value)
	}
	return data, nil
}
