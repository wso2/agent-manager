//
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
//

package utils

type SupportedLanguages string

const (
	LanguageJava      SupportedLanguages = "java"
	LanguagePython    SupportedLanguages = "python"
	LanguageNodeJS    SupportedLanguages = "nodejs"
	LanguageGo        SupportedLanguages = "go"
	LanguagePHP       SupportedLanguages = "php"
	LanguageRuby      SupportedLanguages = "ruby"
	LanguageDotNet    SupportedLanguages = "dotnet"
	LanguageBallerina SupportedLanguages = "ballerina"
)

type BuildPackProviders string

const (
	BuildPackProviderGoogle       BuildPackProviders = "Google"
	BuildPackProviderAMPBallerina BuildPackProviders = "AMP-Ballerina"
)

// Buildpack represents the configuration for a buildpack
type Buildpack struct {
	SupportedVersions  string `json:"supportedVersions"`
	DisplayName        string `json:"displayName"`
	Language           string `json:"language"`
	VersionEnvVariable string `json:"versionEnvVariable"`
	Provider           string `json:"provider"`
}

// Buildpacks contains all supported buildpack configurations
var Buildpacks = []Buildpack{
	{
		SupportedVersions:  "8,11,17,21",
		DisplayName:        "Java",
		Language:           "java",
		VersionEnvVariable: "GOOGLE_RUNTIME_VERSION",
		Provider:           "Google",
	},
	{
		SupportedVersions:  "3.10.x,3.11.x,3.12.x,3.13.x",
		DisplayName:        "Python",
		Language:           "python",
		VersionEnvVariable: "GOOGLE_PYTHON_VERSION",
		Provider:           "Google",
	},
	{
		SupportedVersions:  "12.x.x,14.x.x,16.x.x,18.x.x,20.x.x,22.x.x,24.x.x",
		DisplayName:        "NodeJS",
		Language:           "nodejs",
		VersionEnvVariable: "GOOGLE_NODEJS_VERSION",
		Provider:           "Google",
	},
	{
		SupportedVersions:  "1.x",
		DisplayName:        "Go",
		Language:           "go",
		VersionEnvVariable: "GOOGLE_GO_VERSION",
		Provider:           "Google",
	},
	{
		SupportedVersions:  "8.1.x,8.2.x,8.3.x,8.4.x",
		DisplayName:        "PHP",
		Language:           "php",
		VersionEnvVariable: "GOOGLE_RUNTIME_VERSION",
		Provider:           "Google",
	},
	{
		SupportedVersions:  "3.1.x,3.2.x,3.3.x,3.4.x",
		DisplayName:        "Ruby",
		Language:           "ruby",
		VersionEnvVariable: "GOOGLE_RUNTIME_VERSION",
		Provider:           "Google",
	},
	{
		SupportedVersions:  "6.x,7.x,8.x",
		DisplayName:        ".NET",
		Language:           "dotnet",
		VersionEnvVariable: "GOOGLE_RUNTIME_VERSION",
		Provider:           "Google",
	},
	{
		SupportedVersions:  "",
		DisplayName:        "Ballerina",
		Language:           "ballerina",
		VersionEnvVariable: "",
		Provider:           "AMP-Ballerina",
	},
}
