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

package requests

import (
	"fmt"
)

type HttpError struct {
	StatusCode int
	Body       string
	err        error
}

func (e *HttpError) Error() string {
	if e.err != nil {
		return fmt.Sprintf("failed with status code %d and internal error %s", e.StatusCode, e.err)
	}
	if e.Body == "" {
		return fmt.Sprintf("failed with status code %d", e.StatusCode)
	}
	return fmt.Sprintf("failed with status code %d [%s]", e.StatusCode, e.Body)
}
