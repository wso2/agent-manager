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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// HttpRequest represents a retryable HTTP request.
type HttpRequest struct {
	// Name is a human-readable name for the request. To be used in logs.
	// e.g. "packageName.methodName"
	Name string

	URL    string
	Method string
	Query  map[string]string

	headers   http.Header
	body      []byte
	createErr error
}

func (r *HttpRequest) SetHeader(key, value string) *HttpRequest {
	if r.headers == nil {
		r.headers = http.Header{}
	}
	r.headers.Set(key, value)
	return r
}

func (r *HttpRequest) SetQuery(key, value string) *HttpRequest {
	if r.Query == nil {
		r.Query = make(map[string]string)
	}
	r.Query[key] = value
	return r
}

func (r *HttpRequest) SetJson(body any) *HttpRequest {
	v, err := json.Marshal(body)
	if err != nil {
		r.createErr = fmt.Errorf("failed to encode request body: %w", err)
		return r
	}
	r.body = v
	r.SetHeader("Content-Type", "application/json")
	return r
}

func (r *HttpRequest) buildHttpRequest(ctx context.Context) (*http.Request, error) {
	if r.createErr != nil {
		return nil, r.createErr
	}
	request, err := http.NewRequestWithContext(ctx, r.Method, r.URL, bytes.NewReader(r.body))
	if err != nil {
		return nil, err
	}
	q := request.URL.Query()
	for key, value := range r.Query {
		q.Add(key, value)
	}
	request.URL.RawQuery = q.Encode()
	request.Header = r.headers
	return request, nil
}
