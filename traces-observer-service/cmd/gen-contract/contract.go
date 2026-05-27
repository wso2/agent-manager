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

// Package main contains the gen-contract codegen tool. It walks an in-Go
// description of what the observer's per-kind extractors read and emits the
// JSON Schema bundle consumed by the instrumentation-matrix test suite. See
// attribute-map.md for the rationale behind each Required marker.
package main

// AttributeSpec describes a single attribute key the observer reads on a span.
type AttributeSpec struct {
	Key      string
	Type     string // "string" | "integer" | "number" | "boolean"
	Required bool
	Const    string // optional const value (e.g. traceloop.span.kind = "llm")
	MinLen   int    // optional minLength for strings
	Min      *int   // optional minimum for integers
}

// KindSpec describes one AMP span kind's contract: the keys the observer
// reads and which of those are required (classification trigger or
// downstream-essential).
type KindSpec struct {
	Kind       string
	Attributes []AttributeSpec
}

// Contract is the in-memory snapshot of the AMP span contract. Mirrors what
// opensearch/process.go and crewai_process.go read; see attribute-map.md for
// the per-kind rationale.
var Contract = []KindSpec{
	{
		// Vendor expressed via VendorAnyOf below (legacy gen_ai.system OR
		// current OTel gen_ai.provider.name; the observer's extractVendor
		// helper accepts either).
		Kind: "llm",
		Attributes: []AttributeSpec{
			{Key: "gen_ai.operation.name", Type: "string", Required: true, MinLen: 1},
			{Key: "gen_ai.system", Type: "string"},
			{Key: "gen_ai.provider.name", Type: "string"},
			{Key: "gen_ai.request.model", Type: "string", Required: true, MinLen: 1},
			{Key: "gen_ai.response.model", Type: "string"},
			{Key: "gen_ai.request.temperature", Type: "number"},
			{Key: "gen_ai.usage.input_tokens", Type: "integer", Required: true, Min: ptr(0)},
			{Key: "gen_ai.usage.output_tokens", Type: "integer", Required: true, Min: ptr(0)},
			{Key: "gen_ai.usage.cache_read_input_tokens", Type: "integer", Min: ptr(0)},
		},
	},
	{
		Kind: "embedding",
		Attributes: []AttributeSpec{
			{Key: "gen_ai.operation.name", Type: "string", Required: true, MinLen: 1},
			{Key: "gen_ai.system", Type: "string"},
			{Key: "gen_ai.provider.name", Type: "string"},
			{Key: "gen_ai.request.model", Type: "string", Required: true, MinLen: 1},
			{Key: "gen_ai.response.model", Type: "string"},
			{Key: "gen_ai.usage.input_tokens", Type: "integer", Min: ptr(0)},
		},
	},
	{
		// Tool spans accept any of three name keys (traceloop.entity.name,
		// function.name, gen_ai.tool.name). JSON Schema `required` is AND-only,
		// so none of them are marked Required here; the codegen emits an
		// `anyOf` group on the attributes object below.
		Kind: "tool",
		Attributes: []AttributeSpec{
			{Key: "traceloop.entity.name", Type: "string"},
			{Key: "function.name", Type: "string"},
			{Key: "gen_ai.tool.name", Type: "string"},
			{Key: "traceloop.entity.input", Type: "string"},
			{Key: "traceloop.entity.output", Type: "string"},
			{Key: "gen_ai.tool.status", Type: "string"},
			{Key: "traceloop.span.kind", Type: "string", Const: "tool"},
		},
	},
	{
		Kind: "retriever",
		Attributes: []AttributeSpec{
			{Key: "db.system", Type: "string"},
			{Key: "db.system.name", Type: "string"},
			{Key: "db.collection.name", Type: "string"},
			{Key: "db.vector.query.top_k", Type: "integer", Min: ptr(1)},
		},
	},
	{
		Kind: "rerank",
		Attributes: []AttributeSpec{
			{Key: "traceloop.span.kind", Type: "string", Const: "rerank"},
		},
	},
	{
		Kind: "agent",
		Attributes: []AttributeSpec{
			{Key: "gen_ai.agent.name", Type: "string"},
			{Key: "gen_ai.agent.tools", Type: "string"},
			{Key: "gen_ai.request.model", Type: "string"},
			{Key: "gen_ai.system", Type: "string"},
			{Key: "gen_ai.provider.name", Type: "string"},
			{Key: "gen_ai.conversation.id", Type: "string"},
			// Traceloop 0.60's CrewAI instrumentation stringifies all crewai.*
			// attributes — see FINDINGS.md (F-001).
			{Key: "crewai.agent.max_iter", Type: "string"},
		},
	},
	{
		// AMP-only abstraction. Traceloop emits `workflow` or `task` for spans
		// that classify here, so we don't pin the attribute to a single value.
		Kind: "chain",
		Attributes: []AttributeSpec{
			{Key: "traceloop.span.kind", Type: "string"},
		},
	},
	{
		// CrewAI's Task class has no `name` field — tasks are identified by
		// `description`. The observer's CrewAITaskData.Name has no upstream
		// source; treat it as a contract bug (F-002 in FINDINGS.md). The
		// schema declares the actually-emitted attributes: description and
		// tools. All stringified by Traceloop 0.60.
		Kind: "crewaitask",
		Attributes: []AttributeSpec{
			{Key: "crewai.task.description", Type: "string", Required: true, MinLen: 1},
			{Key: "crewai.task.tools", Type: "string"},
		},
	},
	{
		Kind: "unknown",
		// Permissive — no constraints beyond the envelope.
		Attributes: nil,
	},
}

// ToolNameAnyOf lists the keys at least one of which a tool span must carry
// for the observer's ExtractToolExecutionDetails to find a name. Emitted as a
// JSON Schema `anyOf` clause on the tool kind's attributes object.
var ToolNameAnyOf = []string{
	"traceloop.entity.name",
	"function.name",
	"gen_ai.tool.name",
}

// RetrieverVectorDBAnyOf lists the keys at least one of which a retriever span
// must carry for `RetrieverData.VectorDB` to be populated.
var RetrieverVectorDBAnyOf = []string{
	"db.system",
	"db.system.name",
}

// VendorAnyOf lists the vendor keys the observer's extractVendor helper
// accepts: the legacy gen_ai.system or the current OTel gen_ai.provider.name.
// Applied to the LLM kind. Embedding kind intentionally omits this anyOf —
// see the case statement in renderKindSchema for the upstream gap rationale.
var VendorAnyOf = []string{
	"gen_ai.system",
	"gen_ai.provider.name",
}

func ptr(i int) *int { return &i }
