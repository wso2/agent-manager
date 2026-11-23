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
package config

// aliases maps a setting to the older name it was read by, which still works
// when the setting itself is unset.
var aliases = map[string]string{
	"SERVER_PUBLIC_URL":     "AMP_PUBLIC_URL",
	"THUNDER_BASE_URL":      "THUNDER_PUBLIC_URL",
	"IDP_CLIENT_ID":         "AMP_API_CLIENT_ID",
	"IDP_CLIENT_SECRET":     "AMP_API_CLIENT_SECRET",
	"THUNDER_CLIENT_ID":     "AMP_SYSTEM_CLIENT_ID",
	"THUNDER_CLIENT_SECRET": "AMP_SYSTEM_CLIENT_SECRET",
}
