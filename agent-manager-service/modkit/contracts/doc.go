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

// Package contracts holds the capability interfaces modules provide to each
// other. Each capability lives in its own versioned subpackage (for example
// contracts/llmv1 for "llm.proxy/v1") that declares its id and its Go
// interface. A consumer resolves it with modkit.Lookup; neither side imports
// the other module.
package contracts
