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

package models

import "time"

// A2ACatalogKind narrows the catalogue to one way an agent is reached.
type A2ACatalogKind string

const (
	// A2ACatalogKindPlatform keeps agents with a row whose card is fetched through the gateway.
	A2ACatalogKindPlatform A2ACatalogKind = "platform"
	// A2ACatalogKindExternal keeps agents with a row whose card comes from a registered URL.
	A2ACatalogKindExternal A2ACatalogKind = "external"
	// A2ACatalogKindProxied keeps agents with a row whose environment has a proxy.
	A2ACatalogKindProxied A2ACatalogKind = "proxied"
)

// A2ACatalogFilter is one catalogue page request; zero fields mean "no filter"; Limit 0 means no limit.
type A2ACatalogFilter struct {
	OUID        string
	Query       string
	Project     string
	Environment string
	Kind        A2ACatalogKind
	Status      A2AAgentCardStatus
	Limit       int
	Offset      int
}

// A2ACatalogRow is one card row with its catalogue flags.
type A2ACatalogRow struct {
	A2AAgentCard `gorm:"embedded"`
	// Proxied says the row's environment has an A2A proxy.
	Proxied bool `gorm:"column:proxied"`
	// Matched says the row itself satisfies the search query; always false without one.
	Matched bool `gorm:"column:matched"`
}

// A2ACatalogEnvironment is one environment of a catalogue entry.
type A2ACatalogEnvironment struct {
	Name      string
	Status    A2AAgentCardStatus
	FetchedAt *time.Time
	Proxied   bool
	// CardDiffers says this environment's stored card fingerprint differs from the lead environment's.
	CardDiffers bool
	Matched     bool
}

// A2ACatalogEntry is one agent in the catalogue, summarised from its lead environment's card.
type A2ACatalogEntry struct {
	ProjectName     string
	AgentName       string
	Kind            A2AAgentCardSource
	Name            string
	Description     string
	Version         string
	Skills          []string
	LeadEnvironment string
	// Environments are in the project's pipeline order.
	Environments []A2ACatalogEnvironment
}

// A2ACatalogPage is one page of catalogue entries; Total counts every matching agent.
type A2ACatalogPage struct {
	Entries []A2ACatalogEntry
	Total   int
	Limit   int
	Offset  int
}
