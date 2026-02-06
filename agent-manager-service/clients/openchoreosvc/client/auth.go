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

import "context"

// AuthProvider provides authentication tokens for OpenChoreo API calls.
type AuthProvider interface {
	// GetToken returns a valid access token, refreshing if necessary.
	// Implementations should cache tokens and handle expiration.
	GetToken(ctx context.Context) (string, error)

	// InvalidateToken forces the provider to fetch a new token on the next GetToken call.
	// Use this when a request fails with 401 Unauthorized.
	InvalidateToken()
}
