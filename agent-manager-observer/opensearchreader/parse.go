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

package opensearchreader

import (
	"strings"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
)

type hitList struct {
	Hits struct {
		Hits []struct {
			Source map[string]any `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
}

func (h hitList) first() map[string]any {
	if len(h.Hits.Hits) == 0 {
		return nil
	}
	return h.Hits.Hits[0].Source
}

type bucket struct {
	Key      string  `json:"key"`
	DocCount int     `json:"doc_count"`
	Earliest hitList `json:"earliest_span"`
	RootSpan struct {
		DocCount int     `json:"doc_count"`
		Hit      hitList `json:"hit"`
	} `json:"root_span"`
	Latest hitList `json:"latest_span"`
}

type listResponse struct {
	Took         int `json:"took"`
	Aggregations struct {
		TraceCount struct {
			Distinct struct {
				Value int `json:"value"`
			} `json:"distinct_traces"`
		} `json:"trace_count"`
		Traces struct {
			Buckets []bucket `json:"buckets"`
		} `json:"traces"`
	} `json:"aggregations"`
}

func parseTime(v any) time.Time {
	s, _ := v.(string)
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

func (b bucket) toTraceInfo() observer.TraceInfo {
	tr := observer.TraceInfo{TraceID: b.Key, SpanCount: b.DocCount}
	if src := b.Earliest.first(); src != nil {
		tr.StartTime = parseTime(src["startTime"])
	}
	if src := b.RootSpan.Hit.first(); src != nil {
		tr.RootSpanID, _ = src["spanId"].(string)
		tr.RootSpanName, _ = src["name"].(string)
		tr.TraceName = tr.RootSpanName
		tr.RootSpanKind, _ = src["kind"].(string)
	}
	if src := b.Latest.first(); src != nil {
		tr.EndTime = parseTime(src["endTime"])
	}
	if !tr.StartTime.IsZero() && !tr.EndTime.IsZero() {
		tr.DurationNs = tr.EndTime.Sub(tr.StartTime).Nanoseconds()
	}
	return tr
}

// spanDoc is one stored span.
// ASSUMPTION: these field names match the stored document; confirm with
//
//	osq "otel-traces-*/_search?size=1&pretty"
type spanDoc struct {
	TraceID      string    `json:"traceId"`
	SpanID       string    `json:"spanId"`
	ParentSpanID string    `json:"parentSpanId"`
	Name         string    `json:"name"`
	Kind         string    `json:"kind"`
	StartTime    time.Time `json:"startTime"`
	EndTime      time.Time `json:"endTime"`
	Status       struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"status"`
	Attributes map[string]any `json:"attributes"`
	Resource   map[string]any `json:"resource"`
}

type spansResponse struct {
	Took int `json:"took"`
	Hits struct {
		Total struct {
			Value int `json:"value"`
		} `json:"total"`
		Hits []struct {
			Source spanDoc `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
}

func (d spanDoc) toSpanInfo(includeAttrs bool) observer.SpanInfo {
	start, end := d.StartTime.UTC(), d.EndTime.UTC()
	sp := observer.SpanInfo{
		SpanID: d.SpanID, SpanName: d.Name, ParentSpanID: d.ParentSpanID,
		StartTime: start, EndTime: end, DurationNs: end.Sub(start).Nanoseconds(),
		Kind:   d.Kind,
		Status: &observer.SpanStatus{Code: strings.ToLower(d.Status.Code), Message: d.Status.Message},
	}
	if includeAttrs {
		sp.Attributes = d.Attributes
		sp.ResourceAttributes = d.Resource
	}
	return sp
}
