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

// Cost and behaviour baseline for the trace list and export. Each scenario
// records its upstream calls and response bytes by kind, the call order for
// one trace, a SHA-256 of its response, and its score lookups, and checks
// them against costGoldens. A golden change needs a reason in the PR. Regenerate the
// goldens only with -update:
//
//	go test ./controllers -run TestTraceCostBaseline -update
//
// With -v the test prints the table as markdown for the PR.

package controllers

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"maps"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
)

// updateCostGoldens rewrites costGoldens from this run.
var updateCostGoldens = flag.Bool("update", false, "rewrite the goldens in trace_cost_baseline_test.go")

// costTraces is the baseline fixture's trace count, past the examine cap.
const costTraces = 600

// costGoldens holds each scenario's expected row.
var costGoldens = map[string]costRow{
	// golden:begin
	"list default limit=10": {
		Calls:  kindCounts{Traces: 1, Root: 10, Detail: 30, Spans: 10, AttrSpans: 0},
		Bytes:  kindCounts{Traces: 2303, Root: 2768, Detail: 9658, Spans: 8301, AttrSpans: 0},
		Order:  "trace-0000: root, spans, detail×3",
		SHA256: "4dd8fa83b0f59db95e68ee2df1438cd105a4db40f784e1b3259cf466c2270ae4",
	},
	"list default limit=50": {
		Calls:  kindCounts{Traces: 1, Root: 50, Detail: 150, Spans: 50, AttrSpans: 0},
		Bytes:  kindCounts{Traces: 10583, Root: 13784, Detail: 48408, Spans: 40329, AttrSpans: 0},
		Order:  "trace-0000: root, spans, detail×3",
		SHA256: "34f7092d26333753be233e4c29d7d1ff4ad060aa93fe936e2b209fb53969db93",
	},
	"list default limit=10 cursor page": {
		Calls:  kindCounts{Traces: 1, Root: 10, Detail: 30, Spans: 10, AttrSpans: 0},
		Bytes:  kindCounts{Traces: 4373, Root: 2740, Detail: 9690, Spans: 7909, AttrSpans: 0},
		Order:  "trace-0010: root, spans, detail×3",
		SHA256: "2540839472fef881fca296c34832621ae70ef422d5e94b722f4b6fb177c0a4bd",
	},
	"list status=error (5%)": {
		Calls:  kindCounts{Traces: 3, Root: 200, Detail: 30, Spans: 10, AttrSpans: 0},
		Bytes:  kindCounts{Traces: 73149, Root: 55080, Detail: 9629, Spans: 8448, AttrSpans: 0},
		Order:  "trace-0000: root, spans, detail×3",
		SHA256: "41a2ca201e3a9142c0afe5c72cde9420cb52c9648d4de0f7156cc29b4e2e4e4a",
	},
	"list status=ok": {
		Calls:  kindCounts{Traces: 1, Root: 50, Detail: 141, Spans: 47, AttrSpans: 0},
		Bytes:  kindCounts{Traces: 10583, Root: 13784, Detail: 45525, Spans: 37873, AttrSpans: 0},
		Order:  "trace-0001: root, spans, detail×3",
		SHA256: "59646b8209f2b999aa347b232c37c1668f68ddefbbea459841ee4622f0cab43c",
	},
	"list minDurationMs=800 (20%)": {
		Calls:  kindCounts{Traces: 1, Root: 10, Detail: 30, Spans: 10, AttrSpans: 0},
		Bytes:  kindCounts{Traces: 10583, Root: 2740, Detail: 9682, Spans: 8252, AttrSpans: 0},
		Order:  "trace-0008: root, spans, detail×3",
		SHA256: "f17637c09708be9e0d663aef65cf4bea018372feb9202b2f9ba09f68d418449e",
	},
	"list minTokens=45 (17.5%)": {
		Calls:  kindCounts{Traces: 2, Root: 100, Detail: 300, Spans: 100, AttrSpans: 0},
		Bytes:  kindCounts{Traces: 31516, Root: 27540, Detail: 96846, Spans: 80658, AttrSpans: 0},
		Order:  "trace-0033: root, spans, detail×3",
		SHA256: "f03fa0ed5b828a70ca5bbf3e0eab92f54d891f5a87f3915b6da82e3289add7a4",
	},
	"list minSpanCount=5 (33%)": {
		Calls:  kindCounts{Traces: 1, Root: 10, Detail: 30, Spans: 10, AttrSpans: 0},
		Bytes:  kindCounts{Traces: 10583, Root: 2768, Detail: 9676, Spans: 10359, AttrSpans: 0},
		Order:  "trace-0000: root, spans, detail×3",
		SHA256: "400e4f3ecf35894359b9df506431810b8a41ee82b7de8c3237f733be0a3a8a99",
	},
	"list model=rare (2%)": {
		Calls:  kindCounts{Traces: 5, Root: 0, Detail: 0, Spans: 0, AttrSpans: 500},
		Bytes:  kindCounts{Traces: 280417, Root: 0, Detail: 0, Spans: 0, AttrSpans: 681454},
		Order:  "trace-0000: attrSpans",
		SHA256: "55c2beb542bdd51ab941c53cb9bbd9fab8ba3b98b305bd6e78dc02dff89623e5",
	},
	"list conversationId on root": {
		Calls:  kindCounts{Traces: 1, Root: 50, Detail: 60, Spans: 20, AttrSpans: 0},
		Bytes:  kindCounts{Traces: 10583, Root: 13784, Detail: 19380, Spans: 16210, AttrSpans: 0},
		Order:  "trace-0020: root, spans, detail×3",
		SHA256: "b9061c8ea11ce0d2d1980d0031407fa644a0e8ae115a6ba44a99e786285381be",
	},
	"list conversationId on leaf only": {
		Calls:  kindCounts{Traces: 1, Root: 50, Detail: 150, Spans: 50, AttrSpans: 0},
		Bytes:  kindCounts{Traces: 10583, Root: 12034, Detail: 53908, Spans: 40329, AttrSpans: 0},
		Order:  "trace-0020: root, spans, detail×3",
		SHA256: "b9061c8ea11ce0d2d1980d0031407fa644a0e8ae115a6ba44a99e786285381be",
	},
	"list status=error model=gpt-4o-mini": {
		Calls:  kindCounts{Traces: 4, Root: 250, Detail: 0, Spans: 0, AttrSpans: 13},
		Bytes:  kindCounts{Traces: 156182, Root: 68864, Detail: 0, Spans: 0, AttrSpans: 18672},
		Order:  "trace-0020: root, attrSpans",
		SHA256: "fbb9947613a9368242d944f305702bbb3c233682918d74ff44fa4d9598d19e1b",
	},
	"list include=models": {
		Calls:  kindCounts{Traces: 1, Root: 0, Detail: 0, Spans: 0, AttrSpans: 10},
		Bytes:  kindCounts{Traces: 2303, Root: 0, Detail: 0, Spans: 0, AttrSpans: 14009},
		Order:  "trace-0000: attrSpans",
		SHA256: "4dd8fa83b0f59db95e68ee2df1438cd105a4db40f784e1b3259cf466c2270ae4",
	},
	"list examine cap (no match)": {
		Calls:  kindCounts{Traces: 5, Root: 500, Detail: 0, Spans: 0, AttrSpans: 0},
		Bytes:  kindCounts{Traces: 280417, Root: 137700, Detail: 0, Spans: 0, AttrSpans: 0},
		Order:  "trace-0000: root",
		SHA256: "6c7bbb26b277bf83e6f0c5457c7b719d0d10b753c0f117064d18cdf13bf4d577",
	},
	"export default limit=10": {
		Calls:  kindCounts{Traces: 1, Root: 0, Detail: 0, Spans: 0, AttrSpans: 10},
		Bytes:  kindCounts{Traces: 2105, Root: 0, Detail: 0, Spans: 0, AttrSpans: 14009},
		Order:  "trace-0000: attrSpans",
		SHA256: "13e5143ba4856a4efa883b6aff202d3f494ce6fb6cbdaec95ab8bebb69af91f0",
	},
	"export status=error": {
		Calls:  kindCounts{Traces: 3, Root: 200, Detail: 30, Spans: 10, AttrSpans: 10},
		Bytes:  kindCounts{Traces: 73149, Root: 55080, Detail: 9629, Spans: 8448, AttrSpans: 14457},
		Order:  "trace-0000: root, spans, detail×3, attrSpans",
		SHA256: "f14177e54ded6b251aa635db10c2814b37a0113f34343b62a4a5f8f09630dd66",
	},
	"export span fetch fails": {
		Calls:  kindCounts{Traces: 1, Root: 0, Detail: 0, Spans: 0, AttrSpans: 11},
		Bytes:  kindCounts{Traces: 2105, Root: 0, Detail: 0, Spans: 0, AttrSpans: 12284},
		Order:  "trace-0003: attrSpans×2",
		SHA256: "f05af5e03842eedec552c40725fd0546d18f6410ac96243af101d13ecd5c900b",
	},
	"list include=tools": {
		Calls:  kindCounts{Traces: 1, Root: 10, Detail: 30, Spans: 10, AttrSpans: 0},
		Bytes:  kindCounts{Traces: 2303, Root: 2768, Detail: 9658, Spans: 8301, AttrSpans: 0},
		Order:  "trace-0000: root, spans, detail×3",
		SHA256: "4b7a50d5cc450be2596216cf92f8b40c73c29d8e375d44acb75ee20b1c47ef33",
	},
	"list include=models,tools": {
		Calls:  kindCounts{Traces: 1, Root: 0, Detail: 0, Spans: 0, AttrSpans: 10},
		Bytes:  kindCounts{Traces: 2303, Root: 0, Detail: 0, Spans: 0, AttrSpans: 14009},
		Order:  "trace-0000: attrSpans",
		SHA256: "4b7a50d5cc450be2596216cf92f8b40c73c29d8e375d44acb75ee20b1c47ef33",
	},
	"list root-complete": {
		Calls:  kindCounts{Traces: 1, Root: 10, Detail: 0, Spans: 0, AttrSpans: 0},
		Bytes:  kindCounts{Traces: 2303, Root: 4918, Detail: 0, Spans: 0, AttrSpans: 0},
		Order:  "trace-0000: root",
		SHA256: "9c49c45cc97ab2e6821a325a963b74f118e6958312f408404a5250190fe5a23c",
	},
	"list root-complete include=tools": {
		Calls:  kindCounts{Traces: 1, Root: 10, Detail: 0, Spans: 10, AttrSpans: 0},
		Bytes:  kindCounts{Traces: 2303, Root: 4918, Detail: 0, Spans: 8301, AttrSpans: 0},
		Order:  "trace-0000: root, spans",
		SHA256: "0e4625783fd9c82a222d96d8520d951b7a59f8d405140e6bf040f44bf91ad7b1",
	},
	"list root-complete include=models,tools": {
		Calls:  kindCounts{Traces: 1, Root: 0, Detail: 0, Spans: 0, AttrSpans: 10},
		Bytes:  kindCounts{Traces: 2303, Root: 0, Detail: 0, Spans: 0, AttrSpans: 16159},
		Order:  "trace-0000: attrSpans",
		SHA256: "03ea4ea487736ecd1753489ab15cc370dcc35556620364699f9d012ae8cbcbd5",
	},
	"list root-complete over cap include=tools": {
		Calls:  kindCounts{Traces: 1, Root: 10, Detail: 0, Spans: 0, AttrSpans: 0},
		Bytes:  kindCounts{Traces: 2325, Root: 4918, Detail: 0, Spans: 0, AttrSpans: 0},
		Order:  "trace-0000: root",
		SHA256: "9243570a8b8ef69d4033514efeca0a4dc9d58117fc731bedfaa9955ea5a74340",
	},
	"list toolError=true (3.3%)": {
		Calls:  kindCounts{Traces: 4, Root: 10, Detail: 30, Spans: 300, AttrSpans: 0},
		Bytes:  kindCounts{Traces: 156182, Root: 2880, Detail: 9633, Spans: 241190, AttrSpans: 0},
		Order:  "trace-0000: spans, root, detail×3",
		SHA256: "ba9e979d02f4e7aacccf745392155e6d5e8c0b9329dd0d68c1fc312d6cd2dc80",
	},
	"list tool=search_issues (33%)": {
		Calls:  kindCounts{Traces: 1, Root: 17, Detail: 51, Spans: 50, AttrSpans: 0},
		Bytes:  kindCounts{Traces: 10583, Root: 4686, Detail: 16450, Spans: 40329, AttrSpans: 0},
		Order:  "trace-0000: spans, root, detail×3",
		SHA256: "54c774a173c39148d1b3b80a2b4a4b7550cc6bcb66baf0e61d85ef70a9e9fb33",
	},
	"list tool=search_issues toolError=true": {
		Calls:  kindCounts{Traces: 4, Root: 10, Detail: 30, Spans: 300, AttrSpans: 0},
		Bytes:  kindCounts{Traces: 156182, Root: 2880, Detail: 9633, Spans: 241190, AttrSpans: 0},
		Order:  "trace-0000: spans, root, detail×3",
		SHA256: "ba9e979d02f4e7aacccf745392155e6d5e8c0b9329dd0d68c1fc312d6cd2dc80",
	},
	"list toolError=true status=error": {
		Calls:  kindCounts{Traces: 5, Root: 500, Detail: 27, Spans: 25, AttrSpans: 0},
		Bytes:  kindCounts{Traces: 280417, Root: 137700, Detail: 8670, Spans: 20728, AttrSpans: 0},
		Order:  "trace-0000: root, spans, detail×3",
		SHA256: "585cb33ef8fe4c61565f763e9c02578196d0eb9ff18c13303080bf45b1049a7b",
	},
	"list toolError=true model=gpt-4o-mini": {
		Calls:  kindCounts{Traces: 4, Root: 0, Detail: 0, Spans: 0, AttrSpans: 400},
		Bytes:  kindCounts{Traces: 156182, Root: 0, Detail: 0, Spans: 0, AttrSpans: 545372},
		Order:  "trace-0030: attrSpans",
		SHA256: "2a27bdd68537d776285b7094ef81f9988bef8d08e974073cfb5fc0fa111d1e49",
	},
	"list toolError=true root-complete": {
		Calls:  kindCounts{Traces: 4, Root: 10, Detail: 0, Spans: 300, AttrSpans: 0},
		Bytes:  kindCounts{Traces: 156182, Root: 5030, Detail: 0, Spans: 241190, AttrSpans: 0},
		Order:  "trace-0000: spans, root",
		SHA256: "9aa387b71aa7d70ab863178217ab23950a10089b4fe980574eeb433ca9b64d9c",
	},
	"list toolError=true over cap": {
		Calls:  kindCounts{Traces: 5, Root: 0, Detail: 0, Spans: 0, AttrSpans: 0},
		Bytes:  kindCounts{Traces: 283125, Root: 0, Detail: 0, Spans: 0, AttrSpans: 0},
		Order:  "trace-0000: none",
		SHA256: "6c7bbb26b277bf83e6f0c5457c7b719d0d10b753c0f117064d18cdf13bf4d577",
	},
	"export toolError=true": {
		Calls:  kindCounts{Traces: 4, Root: 10, Detail: 30, Spans: 300, AttrSpans: 10},
		Bytes:  kindCounts{Traces: 156182, Root: 2880, Detail: 9633, Spans: 241190, AttrSpans: 18053},
		Order:  "trace-0000: spans, root, detail×3, attrSpans",
		SHA256: "468cdc1ba68cdd2c8ce59e58d2ab226b942beea35a23f4e762265153e0009b44",
	},
	"list include=mcpServers": {
		Calls:  kindCounts{Traces: 1, Root: 10, Detail: 34, Spans: 10, AttrSpans: 0},
		Bytes:  kindCounts{Traces: 2303, Root: 2768, Detail: 19728, Spans: 8949, AttrSpans: 0},
		Order:  "trace-0000: root, spans, detail×4",
		SHA256: "50716d09bbcd3572a4f2a481f35c11242f2e8682438db59d8c9afb0e8116efbf",
	},
	"list root-complete include=mcpServers": {
		Calls:  kindCounts{Traces: 1, Root: 10, Detail: 4, Spans: 10, AttrSpans: 0},
		Bytes:  kindCounts{Traces: 2303, Root: 4918, Detail: 10070, Spans: 8949, AttrSpans: 0},
		Order:  "trace-0000: root, spans, detail",
		SHA256: "c55858d57040131256711174a14f0465bca1fff10233bd33ae7666e921cd51a2",
	},
	"list mcpServer=github (16.7%)": {
		Calls:  kindCounts{Traces: 2, Root: 17, Detail: 85, Spans: 100, AttrSpans: 0},
		Bytes:  kindCounts{Traces: 31516, Root: 4714, Detail: 101955, Spans: 86166, AttrSpans: 0},
		Order:  "trace-0000: spans, detail, root, detail×3",
		SHA256: "0d662f2221ce5f94a0ea73c77d63db539fe861e6faa8734efd4c0d544f7283a6",
	},
	"list mcpServer=github no handshake": {
		Calls:  kindCounts{Traces: 5, Root: 0, Detail: 0, Spans: 500, AttrSpans: 0},
		Bytes:  kindCounts{Traces: 280417, Root: 0, Detail: 0, Spans: 402114, AttrSpans: 0},
		Order:  "trace-0000: spans",
		SHA256: "6c7bbb26b277bf83e6f0c5457c7b719d0d10b753c0f117064d18cdf13bf4d577",
	},
	"list mcpServer=github 8 handshakes": {
		Calls:  kindCounts{Traces: 2, Root: 17, Detail: 85, Spans: 100, AttrSpans: 0},
		Bytes:  kindCounts{Traces: 31567, Root: 4714, Detail: 101955, Spans: 125708, AttrSpans: 0},
		Order:  "trace-0000: spans, detail, root, detail×3",
		SHA256: "c64804b094043cd32368c92cf70978c3817da470f529eccd35453cdfea93af84",
	},
	"list mcpServer=github toolError=true": {
		Calls:  kindCounts{Traces: 4, Root: 10, Detail: 40, Spans: 300, AttrSpans: 0},
		Bytes:  kindCounts{Traces: 156182, Root: 2880, Detail: 56673, Spans: 257390, AttrSpans: 0},
		Order:  "trace-0000: spans, detail, root, detail×3",
		SHA256: "a1eaeda1690da759ab2d3bbf9fdc19ebb4aac652d30508bb23bc86b681f99b7b",
	},
	"list mcpServer=github status=error": {
		Calls:  kindCounts{Traces: 5, Root: 500, Detail: 36, Spans: 25, AttrSpans: 0},
		Bytes:  kindCounts{Traces: 280417, Root: 137700, Detail: 51006, Spans: 22186, AttrSpans: 0},
		Order:  "trace-0000: root, spans, detail×4",
		SHA256: "c90249270c6286f14e365f89bc0f00bd3c0e2c6878cae00b6cab5763ed99f489",
	},
	"list mcpServer=atlassian model=claude": {
		Calls:  kindCounts{Traces: 2, Root: 0, Detail: 17, Spans: 0, AttrSpans: 100},
		Bytes:  kindCounts{Traces: 31516, Root: 0, Detail: 5627, Spans: 0, AttrSpans: 221229},
		Order:  "trace-0003: attrSpans, detail",
		SHA256: "b7fcbe4c1d2d4fe3a429e6369da8bc19212a5d132ee8eca7b9eb5d00572c3c6e",
	},
	"list mcpServer=github over cap": {
		Calls:  kindCounts{Traces: 5, Root: 0, Detail: 0, Spans: 0, AttrSpans: 0},
		Bytes:  kindCounts{Traces: 283125, Root: 0, Detail: 0, Spans: 0, AttrSpans: 0},
		Order:  "trace-0000: none",
		SHA256: "6c7bbb26b277bf83e6f0c5457c7b719d0d10b753c0f117064d18cdf13bf4d577",
	},
	"export mcpServer=github": {
		Calls:  kindCounts{Traces: 2, Root: 17, Detail: 85, Spans: 100, AttrSpans: 10},
		Bytes:  kindCounts{Traces: 31516, Root: 4714, Detail: 101955, Spans: 86166, AttrSpans: 64118},
		Order:  "trace-0000: spans, detail, root, detail×3, attrSpans",
		SHA256: "54b7726731efc4f758a9cbf8fe8defd4f3d65e96cc1d47843565b7bb367e0fb0",
	},
	"list mcpServer=github 8 handshakes 8 tools": {
		Calls:  kindCounts{Traces: 2, Root: 17, Detail: 221, Spans: 100, AttrSpans: 0},
		Bytes:  kindCounts{Traces: 31567, Root: 4714, Detail: 445151, Spans: 168310, AttrSpans: 0},
		Order:  "trace-0000: spans, detail×5, root, detail×3",
		SHA256: "2b016a4f96ca71ed184a5d3a426f3023e91f4613a2f808166674e223614440ab",
	},
	"list mcpServer=github handshake cap (no match)": {
		Calls:  kindCounts{Traces: 2, Root: 0, Detail: 100, Spans: 100, AttrSpans: 0},
		Bytes:  kindCounts{Traces: 31516, Root: 0, Detail: 33100, Spans: 119496, AttrSpans: 0},
		Order:  "trace-0000: spans, detail",
		SHA256: "8fb26729def57f0c231da7ac2595fd35655e41b6384cac1dc1a77e4087a5782f",
	},
	"list maxScore=0.5 (5%)": {
		Calls:  kindCounts{Traces: 3, Root: 10, Detail: 30, Spans: 10, AttrSpans: 0},
		Bytes:  kindCounts{Traces: 73149, Root: 3020, Detail: 9629, Spans: 8448, AttrSpans: 0},
		Order:  "trace-0000: root, spans, detail×3",
		SHA256: "41a2ca201e3a9142c0afe5c72cde9420cb52c9648d4de0f7156cc29b4e2e4e4a",
		Scores: scoreCounts{Calls: 4, IDs: 200},
	},
	"list maxScore=0.5 status=error": {
		Calls:  kindCounts{Traces: 3, Root: 10, Detail: 30, Spans: 10, AttrSpans: 0},
		Bytes:  kindCounts{Traces: 73149, Root: 3020, Detail: 9629, Spans: 8448, AttrSpans: 0},
		Order:  "trace-0000: root, spans, detail×3",
		SHA256: "41a2ca201e3a9142c0afe5c72cde9420cb52c9648d4de0f7156cc29b4e2e4e4a",
		Scores: scoreCounts{Calls: 4, IDs: 200},
	},
	"list maxScore=0.5 minDurationMs=800": {
		Calls:  kindCounts{Traces: 5, Root: 0, Detail: 0, Spans: 0, AttrSpans: 0},
		Bytes:  kindCounts{Traces: 280417, Root: 0, Detail: 0, Spans: 0, AttrSpans: 0},
		Order:  "trace-0000: none",
		SHA256: "6c7bbb26b277bf83e6f0c5457c7b719d0d10b753c0f117064d18cdf13bf4d577",
		Scores: scoreCounts{Calls: 10, IDs: 100},
	},
	"list maxScore=0.5 toolError=true": {
		Calls:  kindCounts{Traces: 5, Root: 9, Detail: 27, Spans: 25, AttrSpans: 0},
		Bytes:  kindCounts{Traces: 280417, Root: 2718, Detail: 8670, Spans: 20728, AttrSpans: 0},
		Order:  "trace-0000: spans, root, detail×3",
		SHA256: "585cb33ef8fe4c61565f763e9c02578196d0eb9ff18c13303080bf45b1049a7b",
		Scores: scoreCounts{Calls: 10, IDs: 500},
	},
	"list maxScore=0.5 evaluator=Helpfulness (33%)": {
		Calls:  kindCounts{Traces: 1, Root: 17, Detail: 51, Spans: 17, AttrSpans: 0},
		Bytes:  kindCounts{Traces: 10583, Root: 4686, Detail: 16450, Spans: 17625, AttrSpans: 0},
		Order:  "trace-0000: root, spans, detail×3",
		SHA256: "400e4f3ecf35894359b9df506431810b8a41ee82b7de8c3237f733be0a3a8a99",
		Scores: scoreCounts{Calls: 1, IDs: 50},
	},
	"export maxScore=0.5": {
		Calls:  kindCounts{Traces: 3, Root: 10, Detail: 30, Spans: 10, AttrSpans: 10},
		Bytes:  kindCounts{Traces: 73149, Root: 3020, Detail: 9629, Spans: 8448, AttrSpans: 14457},
		Order:  "trace-0000: root, spans, detail×3, attrSpans",
		SHA256: "f14177e54ded6b251aa635db10c2814b37a0113f34343b62a4a5f8f09630dd66",
		Scores: scoreCounts{Calls: 4, IDs: 200},
	},
	"list evaluator=Helpfulness (100%)": {
		Calls:  kindCounts{Traces: 1, Root: 50, Detail: 150, Spans: 50, AttrSpans: 0},
		Bytes:  kindCounts{Traces: 10583, Root: 13784, Detail: 48408, Spans: 40329, AttrSpans: 0},
		Order:  "trace-0000: root, spans, detail×3",
		SHA256: "f8b6df365ce85afa46d8c168de1f4b06166356f38b53ed1510c2438f98da5105",
		Scores: scoreCounts{Calls: 1, IDs: 50},
	},
	// golden:end
}

// callKind is one kind of upstream call.
type callKind int

const (
	kindTraces    callKind = iota // QueryTraces
	kindRoot                      // GetSpanDetails for the root
	kindDetail                    // GetSpanDetails for any other span
	kindSpans                     // QueryTraceSpans without attributes
	kindAttrSpans                 // QueryTraceSpans with attributes
)

// String names the kind in the order column.
func (k callKind) String() string {
	return [...]string{"traces", "root", "detail", "spans", "attrSpans"}[k]
}

// kindCounts holds a number per call kind.
type kindCounts struct {
	Traces, Root, Detail, Spans, AttrSpans int
}

// add adds n to kind's count.
func (c *kindCounts) add(kind callKind, n int) {
	switch kind {
	case kindTraces:
		c.Traces += n
	case kindRoot:
		c.Root += n
	case kindDetail:
		c.Detail += n
	case kindSpans:
		c.Spans += n
	case kindAttrSpans:
		c.AttrSpans += n
	}
}

// total sums every kind.
func (c kindCounts) total() int {
	return c.Traces + c.Root + c.Detail + c.Spans + c.AttrSpans
}

// literal is c as Go source.
func (c kindCounts) literal() string {
	return fmt.Sprintf("kindCounts{Traces: %d, Root: %d, Detail: %d, Spans: %d, AttrSpans: %d}",
		c.Traces, c.Root, c.Detail, c.Spans, c.AttrSpans)
}

// scoreCounts is a scenario's agent-manager-service score lookups and the trace IDs they sent.
type scoreCounts struct {
	Calls, IDs int
}

// costRow is one scenario's measured cost and response.
type costRow struct {
	Calls kindCounts
	// Bytes sums the JSON size of every response, per kind.
	Bytes kindCounts
	// Order is one trace's per-trace call kinds, in call order.
	Order string
	// SHA256 hashes the response JSON.
	SHA256 string
	// Scores is zero for every scenario without a score filter.
	Scores scoreCounts
}

// meteredCall is one upstream call and its response's JSON size.
type meteredCall struct {
	kind    callKind
	traceID string
	bytes   int
}

// meteredClient records every call the fake answers and passes its result through unchanged.
type meteredClient struct {
	*fakeObserverClient
	t *testing.T
	// roots maps each trace ID to its root span ID.
	roots map[string]string

	mu    sync.Mutex
	calls []meteredCall
}

// meter wraps fake, whose traces must already be set.
func meter(t *testing.T, fake *fakeObserverClient) *meteredClient {
	roots := make(map[string]string, len(fake.traces))
	for _, info := range fake.traces {
		roots[info.TraceID] = info.RootSpanID
	}
	return &meteredClient{fakeObserverClient: fake, t: t, roots: roots}
}

// record logs a call; a failed call returned no bytes.
func (m *meteredClient) record(kind callKind, traceID string, resp any, err error) {
	n := 0
	if err == nil {
		b, mErr := json.Marshal(resp)
		if mErr != nil {
			m.t.Errorf("marshal %s response: %v", kind, mErr)
		}
		n = len(b)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, meteredCall{kind: kind, traceID: traceID, bytes: n})
}

// QueryTraces records the fake's QueryTraces call.
func (m *meteredClient) QueryTraces(ctx context.Context, req observer.TracesQueryRequest) (*observer.TracesQueryResponse, error) {
	resp, err := m.fakeObserverClient.QueryTraces(ctx, req)
	m.record(kindTraces, "", resp, err)
	return resp, err
}

// QueryTraceSpans records the fake's QueryTraceSpans call.
func (m *meteredClient) QueryTraceSpans(ctx context.Context, traceID string, req observer.TracesQueryRequest) (*observer.TraceSpansQueryResponse, error) {
	resp, err := m.fakeObserverClient.QueryTraceSpans(ctx, traceID, req)
	kind := kindSpans
	if req.IncludeAttributes {
		kind = kindAttrSpans
	}
	m.record(kind, traceID, resp, err)
	return resp, err
}

// GetSpanDetails records the fake's GetSpanDetails call.
func (m *meteredClient) GetSpanDetails(ctx context.Context, traceID, spanID string) (*observer.SpanDetailsResponse, error) {
	resp, err := m.fakeObserverClient.GetSpanDetails(ctx, traceID, spanID)
	kind := kindDetail
	if spanID == m.roots[traceID] {
		kind = kindRoot
	}
	m.record(kind, traceID, resp, err)
	return resp, err
}

// row sums the calls and bytes by kind and gives traceID's call order.
func (m *meteredClient) row(traceID string) costRow {
	m.mu.Lock()
	defer m.mu.Unlock()
	var row costRow
	var order []callKind
	for _, c := range m.calls {
		row.Calls.add(c.kind, 1)
		row.Bytes.add(c.kind, c.bytes)
		if c.traceID == traceID {
			order = append(order, c.kind)
		}
	}
	row.Order = traceID + ": " + orderString(order)
	return row
}

// orderString joins kinds, folding a run of one kind into "kind×n".
func orderString(kinds []callKind) string {
	if len(kinds) == 0 {
		return "none"
	}
	var parts []string
	for i := 0; i < len(kinds); {
		j := i + 1
		for j < len(kinds) && kinds[j] == kinds[i] {
			j++
		}
		part := kinds[i].String()
		if j-i > 1 {
			part += fmt.Sprintf("×%d", j-i)
		}
		parts = append(parts, part)
		i = j
	}
	return strings.Join(parts, ", ")
}

// costConversation is trace i's conversation: 20 turns each.
func costConversation(i int) string { return fmt.Sprintf("conv-%02d", i/20) }

// costFake builds the baseline fixture from langGraphFake: costTraces traces,
// every 20th root failed and every 50th trace on rare-model. Trace i lasts
// (i%10)*100 ms. Every 3rd trace calls search_issues through MCP, failing on
// every 30th. The conversation ID sits on the root with convOnRoot, and
// otherwise on every span but the root and chain.
func costFake(convOnRoot bool) *fakeObserverClient {
	fake := withModel(langGraphFake(costTraces, func(i int) map[string]interface{} {
		attrs := map[string]interface{}{}
		maps.Copy(attrs, errorEvery(20)(i))
		if convOnRoot {
			attrs["gen_ai.conversation.id"] = costConversation(i)
		}
		return attrs
	}), 50, "rare-model")
	for i := range fake.traces {
		info := &fake.traces[i]
		d := time.Duration(i%10) * 100 * time.Millisecond
		info.EndTime, info.DurationNs = info.StartTime.Add(d), d.Nanoseconds()
		if i%3 == 0 {
			addToolCall(fake, i, i%30 == 0)
		}
		info.SpanCount = len(fake.spansByTrace[info.TraceID])
	}
	if !convOnRoot {
		setThreads(fake, false, costConversation)
	}
	return fake
}

// addToolCall puts an execute_tool span over an MCP .tool span under trace i's
// chain span, ahead of the root. A failed call errors only the inner span.
func addToolCall(fake *fakeObserverClient, i int, failed bool) {
	info := fake.traces[i]
	spans := fake.spansByTrace[info.TraceID]
	outer := observer.SpanInfo{
		SpanID: fmt.Sprintf("tool-%04d", i), SpanName: "execute_tool search_issues", ParentSpanID: spans[0].SpanID, StartTime: info.StartTime,
		Attributes: map[string]interface{}{
			"gen_ai.operation.name": "execute_tool",
			"gen_ai.tool.name":      "search_issues",
			"gen_ai.task.status":    "success",
		},
	}
	inner := observer.SpanInfo{
		SpanID: fmt.Sprintf("tool-mcp-%04d", i), SpanName: "search_issues.tool", ParentSpanID: outer.SpanID, StartTime: info.StartTime,
		Attributes: map[string]interface{}{
			"traceloop.span.kind":   "tool",
			"traceloop.entity.name": "search_issues",
		},
	}
	if failed {
		inner.Status = &observer.SpanStatus{Code: "error", Message: "tool_error"}
		inner.Attributes["error.type"] = "tool_error"
	}
	fake.spansByTrace[info.TraceID] = slices.Insert(spans, len(spans)-1, outer, inner)
	for _, s := range []observer.SpanInfo{outer, inner} {
		fake.spanDetails[s.SpanID] = &observer.SpanDetailsResponse{
			SpanID: s.SpanID, SpanName: s.SpanName, ParentSpanID: s.ParentSpanID, StartTime: s.StartTime, Status: s.Status, Attributes: s.Attributes,
		}
	}
}

// costScenario is one request the baseline measures.
type costScenario struct {
	name string
	// fixture builds the fake; nil means costFake(true).
	fixture func() *fakeObserverClient
	// params changes lookBackParams(10) without its filter.
	params func(p *TraceQueryParams)
	// export runs ExportTraces instead of GetTraceOverviews.
	export bool
	// cursorPage measures the page after the first.
	cursorPage bool
	// orderTrace is the trace whose call order is recorded; "" means the
	// first trace returned, or trace-0000 when none is.
	orderTrace string
	// fail fails every QueryTraceSpans and GetSpanDetails call for this ID.
	fail string
}

// withFilters sets the request's filters.
func withFilters(f TraceFilters) func(p *TraceQueryParams) {
	return func(p *TraceQueryParams) { p.Filters = f }
}

// costScenarios lists the baseline's scenarios in table order. Add new ones
// at the end.
func costScenarios() []costScenario {
	return []costScenario{
		{name: "list default limit=10"},
		{name: "list default limit=50", params: func(p *TraceQueryParams) { p.Limit = 50 }},
		{name: "list default limit=10 cursor page", cursorPage: true},
		{name: "list status=error (5%)", params: withFilters(TraceFilters{Status: TraceStatusError})},
		{name: "list status=ok", params: withFilters(TraceFilters{Status: TraceStatusOK})},
		{name: "list minDurationMs=800 (20%)", params: withFilters(TraceFilters{MinDurationMs: ptr(800)})},
		{name: "list minTokens=45 (17.5%)", params: withFilters(TraceFilters{MinTokens: ptr(45)})},
		{name: "list minSpanCount=5 (33%)", params: withFilters(TraceFilters{MinSpanCount: ptr(5)})},
		{name: "list model=rare (2%)", params: withFilters(TraceFilters{Model: "rare"})},
		{name: "list conversationId on root", params: withFilters(TraceFilters{ConversationID: "conv-01"})},
		{name: "list conversationId on leaf only", params: withFilters(TraceFilters{ConversationID: "conv-01"}),
			fixture: func() *fakeObserverClient { return costFake(false) }},
		{name: "list status=error model=gpt-4o-mini", params: withFilters(TraceFilters{Status: TraceStatusError, Model: "gpt-4o-mini"})},
		{name: "list include=models", params: func(p *TraceQueryParams) { p.Include.Models = true }},
		{name: "list examine cap (no match)", params: withFilters(TraceFilters{ConversationID: "conv-none"})},
		{name: "export default limit=10", export: true},
		{name: "export status=error", export: true, params: withFilters(TraceFilters{Status: TraceStatusError})},
		{name: "export span fetch fails", export: true, fail: "trace-0003", orderTrace: "trace-0003"},
		{name: "list include=tools", params: includeTools(false)},
		{name: "list include=models,tools", params: includeTools(true)},
		{name: "list root-complete", fixture: costRootCompleteFake},
		{name: "list root-complete include=tools", fixture: costRootCompleteFake, params: includeTools(false)},
		{name: "list root-complete include=models,tools", fixture: costRootCompleteFake, params: includeTools(true)},
		{name: "list root-complete over cap include=tools", fixture: costOverCapFake, params: includeTools(false)},
		{name: "list toolError=true (3.3%)", params: withFilters(TraceFilters{ToolError: true})},
		{name: "list tool=search_issues (33%)", params: withFilters(TraceFilters{Tool: "search_issues"})},
		{name: "list tool=search_issues toolError=true", params: withFilters(TraceFilters{Tool: "search_issues", ToolError: true})},
		{name: "list toolError=true status=error", params: withFilters(TraceFilters{ToolError: true, Status: TraceStatusError})},
		{name: "list toolError=true model=gpt-4o-mini", params: withFilters(TraceFilters{ToolError: true, Model: "gpt-4o-mini"})},
		{name: "list toolError=true root-complete", fixture: costRootCompleteFake, params: withFilters(TraceFilters{ToolError: true})},
		{name: "list toolError=true over cap", fixture: costOverCapFake, params: withFilters(TraceFilters{ToolError: true})},
		{name: "export toolError=true", export: true, params: withFilters(TraceFilters{ToolError: true})},
		{name: "list include=mcpServers", fixture: mcpFake1, params: includeMCPServers},
		{name: "list root-complete include=mcpServers", fixture: mcpRootCompleteFake, params: includeMCPServers},
		{name: "list mcpServer=github (16.7%)", fixture: mcpFake1, params: withFilters(TraceFilters{MCPServer: "github"})},
		{name: "list mcpServer=github no handshake", params: withFilters(TraceFilters{MCPServer: "github"})},
		{name: "list mcpServer=github 8 handshakes", fixture: func() *fakeObserverClient { return mcpFake(8) },
			params: withFilters(TraceFilters{MCPServer: "github"})},
		{name: "list mcpServer=github toolError=true", fixture: mcpFake1, params: withFilters(TraceFilters{MCPServer: "github", ToolError: true})},
		{name: "list mcpServer=github status=error", fixture: mcpFake1, params: withFilters(TraceFilters{MCPServer: "github", Status: TraceStatusError})},
		{name: "list mcpServer=atlassian model=claude", fixture: mcpFake1, params: withFilters(TraceFilters{MCPServer: "atlassian", Model: "claude"})},
		{name: "list mcpServer=github over cap", fixture: costOverCapFake, params: withFilters(TraceFilters{MCPServer: "github"})},
		{name: "export mcpServer=github", fixture: mcpFake1, export: true, params: withFilters(TraceFilters{MCPServer: "github"})},
		{name: "list mcpServer=github 8 handshakes 8 tools", fixture: func() *fakeObserverClient { return mcpToolsFake(8) },
			params: withFilters(TraceFilters{MCPServer: "github"})},
		{name: "list mcpServer=github handshake cap (no match)", fixture: func() *fakeObserverClient { return mcpEveryTraceFake(1, atlassianOnly) },
			params: withFilters(TraceFilters{MCPServer: "github"})},
		{name: "list maxScore=0.5 (5%)", params: withFilters(TraceFilters{MaxScore: f64(0.5)})},
		{name: "list maxScore=0.5 status=error", params: withFilters(TraceFilters{MaxScore: f64(0.5), Status: TraceStatusError})},
		{name: "list maxScore=0.5 minDurationMs=800", params: withFilters(TraceFilters{MaxScore: f64(0.5), MinDurationMs: ptr(800)})},
		{name: "list maxScore=0.5 toolError=true", params: withFilters(TraceFilters{MaxScore: f64(0.5), ToolError: true})},
		{name: "list maxScore=0.5 evaluator=Helpfulness (33%)", params: withFilters(TraceFilters{MaxScore: f64(0.5), Evaluator: "Helpfulness"})},
		{name: "export maxScore=0.5", export: true, params: withFilters(TraceFilters{MaxScore: f64(0.5)})},
		{name: "list evaluator=Helpfulness (100%)", params: withFilters(TraceFilters{Evaluator: "Helpfulness"})},
	}
}

// runCostScenario runs sc against a fresh fixture and measures it.
func runCostScenario(t *testing.T, sc costScenario) costRow {
	t.Helper()
	fixture := sc.fixture
	if fixture == nil {
		fixture = func() *fakeObserverClient { return costFake(true) }
	}
	params := lookBackParams(10)
	params.Filters = TraceFilters{}
	if sc.params != nil {
		sc.params(&params)
	}
	ctx, _ := logContext()
	if sc.cursorPage {
		first, err := NewTracingController(fixture()).GetTraceOverviews(ctx, params)
		if err != nil {
			t.Fatalf("%s: first page: %v", sc.name, err)
		}
		if params.Cursor, err = DecodeTraceCursor(first.NextCursor); err != nil {
			t.Fatalf("%s: first page cursor: %v", sc.name, err)
		}
	}

	fake := fixture()
	if sc.fail != "" {
		fake.failCall = func(id string) error {
			if id == sc.fail {
				return failEvery(id)
			}
			return nil
		}
	}
	client := meter(t, fake)
	scores := &fakeScoreClient{}
	c := NewTracingController(client).WithScoreClient(scores)
	var resp any
	first := "trace-0000"
	if sc.export {
		r, err := c.ExportTraces(ctx, params)
		if err != nil {
			t.Fatalf("%s: ExportTraces: %v", sc.name, err)
		}
		if len(r.Traces) > 0 {
			first = r.Traces[0].TraceID
		}
		resp = r
	} else {
		r, err := c.GetTraceOverviews(ctx, params)
		if err != nil {
			t.Fatalf("%s: GetTraceOverviews: %v", sc.name, err)
		}
		if len(r.Traces) > 0 {
			first = r.Traces[0].TraceID
		}
		resp = r
	}
	body, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("%s: marshal response: %v", sc.name, err)
	}
	sum := sha256.Sum256(body)

	row := client.row(cmp.Or(sc.orderTrace, first))
	row.SHA256 = hex.EncodeToString(sum[:])
	row.Scores = scores.counts()
	return row
}

// Every scenario's calls, bytes, call order and response match its golden row.
func TestTraceCostBaseline(t *testing.T) {
	scenarios := costScenarios()
	rows := make(map[string]costRow, len(scenarios))
	names := make([]string, 0, len(scenarios))
	for _, sc := range scenarios {
		if _, dup := rows[sc.name]; dup {
			t.Fatalf("scenario %q listed twice", sc.name)
		}
		got := runCostScenario(t, sc)
		rows[sc.name] = got
		names = append(names, sc.name)
		if *updateCostGoldens {
			continue
		}
		want, ok := costGoldens[sc.name]
		if !ok {
			t.Errorf("%s: no golden row; add it with -update and give the reason in the PR", sc.name)
			continue
		}
		if got.Calls != want.Calls {
			t.Errorf("%s: calls = %s, want %s", sc.name, got.Calls.literal(), want.Calls.literal())
		}
		if got.Bytes != want.Bytes {
			t.Errorf("%s: bytes = %s, want %s", sc.name, got.Bytes.literal(), want.Bytes.literal())
		}
		if got.Order != want.Order {
			t.Errorf("%s: order = %q, want %q", sc.name, got.Order, want.Order)
		}
		if got.SHA256 != want.SHA256 {
			t.Errorf("%s: response sha256 = %s, want %s", sc.name, got.SHA256, want.SHA256)
		}
		if got.Scores != want.Scores {
			t.Errorf("%s: score lookups = %+v, want %+v", sc.name, got.Scores, want.Scores)
		}
	}
	if !*updateCostGoldens {
		for name := range costGoldens {
			if _, ok := rows[name]; !ok {
				t.Errorf("golden row %q has no scenario", name)
			}
		}
	} else {
		writeCostGoldens(t, names, rows)
	}
	if testing.Verbose() {
		fmt.Print(costTable(names, rows))
	}
}

// writeCostGoldens replaces the costGoldens entries in this file with rows.
func writeCostGoldens(t *testing.T, names []string, rows map[string]costRow) {
	t.Helper()
	const file = "trace_cost_baseline_test.go"
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	begin, end := []byte("// golden:begin\n"), []byte("// golden:end\n")
	i, j := bytes.Index(src, begin), bytes.Index(src, end)
	if i < 0 || j < i {
		t.Fatalf("%s has no golden:begin / golden:end markers", file)
	}
	var b bytes.Buffer
	b.Write(src[:i+len(begin)])
	for _, name := range names {
		r := rows[name]
		fmt.Fprintf(&b, "%q: {\nCalls: %s,\nBytes: %s,\nOrder: %q,\nSHA256: %q,\n",
			name, r.Calls.literal(), r.Bytes.literal(), r.Order, r.SHA256)
		// Rows without score lookups keep their text.
		if r.Scores != (scoreCounts{}) {
			fmt.Fprintf(&b, "Scores: scoreCounts{Calls: %d, IDs: %d},\n", r.Scores.Calls, r.Scores.IDs)
		}
		b.WriteString("},\n")
	}
	b.Write(src[j:])
	out, err := format.Source(b.Bytes())
	if err != nil {
		t.Fatalf("format %s: %v", file, err)
	}
	if err := os.WriteFile(file, out, 0o600); err != nil {
		t.Fatalf("write %s: %v", file, err)
	}
}

// costTable renders rows as a markdown table; each kind cell is calls / bytes.
func costTable(names []string, rows map[string]costRow) string {
	cell := func(calls, n int) string {
		if calls == 0 {
			return "0"
		}
		return fmt.Sprintf("%d / %d B", calls, n)
	}
	var b strings.Builder
	scoreCell := func(c scoreCounts) string {
		if c.Calls == 0 {
			return "0"
		}
		return fmt.Sprintf("%d / %d IDs", c.Calls, c.IDs)
	}
	b.WriteString("\n| Scenario | QueryTraces | Root details | Other details | Span lists | Attribute span lists | Total | Call order, one trace | Response SHA-256 | Score lookups |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|---|---|\n")
	for _, name := range names {
		r := rows[name]
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s | `%s` | %s |\n", name,
			cell(r.Calls.Traces, r.Bytes.Traces),
			cell(r.Calls.Root, r.Bytes.Root),
			cell(r.Calls.Detail, r.Bytes.Detail),
			cell(r.Calls.Spans, r.Bytes.Spans),
			cell(r.Calls.AttrSpans, r.Bytes.AttrSpans),
			cell(r.Calls.total(), r.Bytes.total()),
			r.Order, r.SHA256[:12], scoreCell(r.Scores))
	}
	return b.String()
}
