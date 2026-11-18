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

// Package gatewaytoken is the format of a gateway's control-plane registration
// token: a UUID, which is the stored row's lookup prefix, then a dash and 32
// random bytes, base64url without padding.
package gatewaytoken

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/google/uuid"
)

// randomLen is the length of the encoded random part.
var randomLen = base64.RawURLEncoding.EncodedLen(32)

// New returns a fresh token.
func New() (string, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", errors.New("failed to generate secure random bytes")
	}
	return uuid.New().String() + "-" + base64.RawURLEncoding.EncodeToString(random), nil
}

// Prefix is the token's UUID prefix, or an error when the token is not in the format New returns.
func Prefix(token string) (string, error) {
	if len(token) != 36+1+randomLen || token[36] != '-' {
		return "", errors.New("token must be a UUID, a dash and 43 base64url characters")
	}
	prefix := token[:36]
	if _, err := uuid.Parse(prefix); err != nil {
		return "", errors.New("token must start with a UUID")
	}
	if _, err := base64.RawURLEncoding.DecodeString(token[37:]); err != nil || strings.ContainsAny(token[37:], "=") {
		return "", errors.New("token must end with 43 base64url characters")
	}
	return prefix, nil
}
