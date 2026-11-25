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

package amctl

import (
	. "github.com/onsi/gomega"
)

// Login runs the real `amctl login` with the client_credentials grant, using
// the same IDP client the HTTP suite uses. Scopes come from OIDC discovery.
// --url requires a context name; without --url amctl would log in to WSO2 Cloud.
func (h *Harness) Login(g Gomega) {
	res := h.Run("login",
		"--url", h.cfg.AMPBaseURL,
		"--name", "e2e",
		"--client-id", h.cfg.IDPClientID,
		"--client-secret", h.cfg.IDPClientSecret,
		"--json",
	)
	g.Expect(res.ExitCode).To(Equal(0), "amctl login failed: %s", res.Combined())
}
