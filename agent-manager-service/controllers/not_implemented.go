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
	"errors"
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// writeIfNotImplemented answers 501 for an operation that needs a capability
// this deployment does not provide, and reports whether it wrote a response.
func writeIfNotImplemented(w http.ResponseWriter, err error) bool {
	if !errors.Is(err, utils.ErrNotImplemented) {
		return false
	}
	utils.WriteErrorResponse(w, http.StatusNotImplemented, err.Error())
	return true
}
