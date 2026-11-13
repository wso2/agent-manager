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

package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/orgctx"
)

func TestWithOrgContext(t *testing.T) {
	bare := context.Background()
	named := withOrgContext(bare, "org-a")
	org, ok := orgctx.GetResolvedOrg(named)
	require.True(t, ok)
	require.Equal(t, "org-a", org.OUID)

	withHandle := orgctx.WithResolvedOrg(bare, orgctx.ResolvedOrg{OUID: "org-a", OuHandle: "acme"})
	require.Equal(t, withHandle, withOrgContext(withHandle, "org-a"), "the same organization leaves the context as it is")

	switched := withOrgContext(withHandle, "org-b")
	org, _ = orgctx.GetResolvedOrg(switched)
	require.Equal(t, orgctx.ResolvedOrg{OUID: "org-b"}, org, "another organization is named, never mixed with the old handle")
}
