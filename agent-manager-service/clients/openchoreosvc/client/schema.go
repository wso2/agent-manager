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
	_ "embed"
	"fmt"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

//go:embed default-openapi-schema.yaml
var defaultChatAPISchema string

// ballerinaChatAPISchema is the contract a ballerina/ai ai:Listener serves:
// POST <basePath>/chat {message, sessionId} -> {message}.
//
//go:embed ballerina-chat-openapi-schema.yaml
var ballerinaChatAPISchema string

func getDefaultChatAPISchema() (string, error) {
	if defaultChatAPISchema == "" {
		return "", fmt.Errorf("failed to read chat API schema: embedded schema is empty")
	}
	return defaultChatAPISchema, nil
}

// getChatAPISchema returns the chat-api OpenAPI for an agent of the given
// buildpack language: the ai:Listener contract for Ballerina, the platform's
// default chat contract otherwise.
func getChatAPISchema(language string) (string, error) {
	if language == string(utils.LanguageBallerina) {
		if ballerinaChatAPISchema == "" {
			return "", fmt.Errorf("failed to read Ballerina chat API schema: embedded schema is empty")
		}
		return ballerinaChatAPISchema, nil
	}
	return getDefaultChatAPISchema()
}

// BallerinaChatAPIDefaultPort is the port a ballerina/ai ai:Listener listens on
// unless the program configures another one.
const BallerinaChatAPIDefaultPort int32 = 9090

// ChatAPIDefaultPort is the port a chat-api agent serves on when its input
// interface names none: 9090 for a Ballerina (ai:Listener) agent, the platform's
// configured chat port otherwise.
func ChatAPIDefaultPort(language string) int32 {
	if language == string(utils.LanguageBallerina) {
		return BallerinaChatAPIDefaultPort
	}
	return config.GetConfig().DefaultChatAPI.DefaultHTTPPort
}

// buildLanguage returns the buildpack language of a build, or "" for docker or
// unknown builds.
func buildLanguage(build *BuildConfig) string {
	if build == nil || build.Buildpack == nil {
		return ""
	}
	return build.Buildpack.Language
}
