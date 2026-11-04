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

//go:build devstore

package devstore

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
)

type recordedLogQuery struct {
	limit int
	desc  bool
}

type recordingLogs struct{ got *recordedLogQuery }

func (r *recordingLogs) Query(_ context.Context, _ observer.LogsQueryRequest, limit int, desc bool) (*observer.LogsQueryResponse, error) {
	r.got = &recordedLogQuery{limit: limit, desc: desc}
	return &observer.LogsQueryResponse{}, nil
}

func newTestClient(t *testing.T) (*Client, *recordingLogs) {
	t.Helper()
	store, err := OpenSpanStore(filepath.Join(t.TempDir(), "traces.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	logs := &recordingLogs{}
	dyn := newFakeDynamic(ocObject("Component", "a", "c1"), ocObject("Component", "b", "c2"))
	return &Client{
		spans:            store,
		resolver:         NewResolver(dyn, DefaultResolverTTL, time.Now),
		logs:             logs,
		defaultNamespace: "default",
	}, logs
}

func componentScope(name string) observer.ComponentSearchScope {
	return observer.ComponentSearchScope{Namespace: testNamespace, Component: strPtr(name)}
}

func windowRequest(name string) observer.TracesQueryRequest {
	return observer.TracesQueryRequest{
		StartTime:   time.Unix(0, 0),
		EndTime:     time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC),
		SearchScope: componentScope(name),
	}
}

func ingestSpan(t *testing.T, c *Client) {
	t.Helper()
	h := NewIngestHandler(c.spans, DefaultMaxIngestBytes)
	rec := post(h, buildRequest(t, nil), protobufType, "", map[string]string{"x-user-component": "c1", "x-user-namespace": testNamespace})
	if rec.Code != http.StatusOK {
		t.Fatalf("ingest status %d: %s", rec.Code, rec.Body)
	}
}

const (
	otelTraceID = "0102030405060708090a0b0c0d0e0f10"
	otelSpanID  = "0102030405060708"
)

func TestClientEndToEndIngestThenQuery(t *testing.T) {
	c, _ := newTestClient(t)
	ingestSpan(t, c)
	ctx := context.Background()

	traces, err := c.QueryTraces(ctx, windowRequest("a"))
	if err != nil {
		t.Fatal(err)
	}
	if len(traces.Traces) != 1 || traces.Total != 1 {
		t.Fatalf("traces = %+v, want 1", traces)
	}
	spans, err := c.QueryTraceSpans(ctx, otelTraceID, windowRequest("a"))
	if err != nil {
		t.Fatal(err)
	}
	if len(spans.Spans) != 1 || spans.Total != 1 {
		t.Fatalf("spans = %+v, want 1", spans)
	}
	other, err := c.QueryTraces(ctx, windowRequest("b"))
	if err != nil {
		t.Fatal(err)
	}
	if len(other.Traces) != 0 {
		t.Fatalf("other agent traces = %+v, want none", other.Traces)
	}
}

func TestClientUnknownAgentIsEmpty(t *testing.T) {
	c, _ := newTestClient(t)
	ingestSpan(t, c)

	traces, err := c.QueryTraces(context.Background(), windowRequest("missing"))
	if err != nil {
		t.Fatal(err)
	}
	if traces.Traces == nil || len(traces.Traces) != 0 {
		t.Fatalf("traces = %#v, want empty non-nil slice", traces.Traces)
	}
	spans, err := c.QueryTraceSpans(context.Background(), otelTraceID, windowRequest("missing"))
	if err != nil {
		t.Fatal(err)
	}
	if spans.Spans == nil || len(spans.Spans) != 0 {
		t.Fatalf("spans = %#v, want empty non-nil slice", spans.Spans)
	}
}

func TestClientGetSpanDetailsNotFoundIsError(t *testing.T) {
	c, _ := newTestClient(t)
	_, err := c.GetSpanDetails(context.Background(), "nope", "nope")
	if !errors.Is(err, ErrSpanNotFound) {
		t.Fatalf("err = %v, want ErrSpanNotFound", err)
	}
}

func TestClientGetSpanDetailsFound(t *testing.T) {
	c, _ := newTestClient(t)
	ingestSpan(t, c)
	d, err := c.GetSpanDetails(context.Background(), otelTraceID, otelSpanID)
	if err != nil {
		t.Fatal(err)
	}
	if d.SpanName != "root" {
		t.Fatalf("span name = %q, want root", d.SpanName)
	}
}

func TestClientQueryMetricsEmpty(t *testing.T) {
	c, _ := newTestClient(t)
	got, err := c.QueryMetrics(context.Background(), observer.MetricsQueryRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if *got != (observer.ResourceMetricsTimeSeries{}) {
		t.Fatalf("metrics = %+v, want zero value", got)
	}
}

func TestClientNamespaceFor(t *testing.T) {
	c, _ := newTestClient(t)
	if got := c.NamespaceFor("any-org"); got != "default" {
		t.Fatalf("namespace = %q, want default", got)
	}
}

func TestClientDefaultsLimitAndSort(t *testing.T) {
	c, logs := newTestClient(t)
	var spans []Span
	for i := 0; i < 25; i++ {
		id := fmt.Sprintf("%032x", i)
		spans = append(spans, Span{
			TraceID: id, SpanID: fmt.Sprintf("%016x", i), Name: "root",
			StartNs: int64(1000 + i), EndNs: int64(2000 + i), ComponentUID: "c1", Namespace: testNamespace,
			Attributes: map[string]any{}, ResourceAttributes: map[string]string{},
		})
	}
	if err := c.spans.Insert(context.Background(), spans); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	traces, err := c.QueryTraces(ctx, windowRequest("a"))
	if err != nil {
		t.Fatal(err)
	}
	if len(traces.Traces) != 20 || traces.Total != 25 {
		t.Fatalf("traces = %d, total = %d, want 20 of 25", len(traces.Traces), traces.Total)
	}
	if traces.Traces[0].TraceID != fmt.Sprintf("%032x", 24) {
		t.Fatalf("first trace = %s, want newest first", traces.Traces[0].TraceID)
	}

	asc := "asc"
	req := windowRequest("a")
	req.SortOrder = &asc
	ascending, err := c.QueryTraces(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if ascending.Traces[0].TraceID != fmt.Sprintf("%032x", 0) {
		t.Fatalf("first trace = %s, want oldest first", ascending.Traces[0].TraceID)
	}

	if _, err := c.QueryLogs(ctx, observer.LogsQueryRequest{}); err != nil {
		t.Fatal(err)
	}
	if *logs.got != (recordedLogQuery{limit: 100, desc: true}) {
		t.Fatalf("log query = %+v, want limit 100 desc", *logs.got)
	}
}

func TestClientSpanDefaultsLimitAndSort(t *testing.T) {
	c, _ := newTestClient(t)
	var spans []Span
	for i := 0; i < 25; i++ {
		spans = append(spans, Span{
			TraceID: otelTraceID, SpanID: fmt.Sprintf("%016x", i), ParentSpanID: "", Name: "s",
			StartNs: int64(1000 + i), EndNs: int64(2000 + i), ComponentUID: "c1", Namespace: testNamespace,
			Attributes: map[string]any{}, ResourceAttributes: map[string]string{},
		})
	}
	if err := c.spans.Insert(context.Background(), spans); err != nil {
		t.Fatal(err)
	}
	got, err := c.QueryTraceSpans(context.Background(), otelTraceID, windowRequest("a"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Spans) != 20 || got.Total != 25 {
		t.Fatalf("spans = %d, total = %d, want 20 of 25", len(got.Spans), got.Total)
	}
	if got.Spans[0].SpanID != fmt.Sprintf("%016x", 24) {
		t.Fatalf("first span = %s, want newest first", got.Spans[0].SpanID)
	}
}

func TestNewReturnsClientIngestAndCloser(t *testing.T) {
	prevPath, prevAddr, prevKube := dbPath, ingestAddr, newKubeClients
	t.Cleanup(func() { dbPath, ingestAddr, newKubeClients = prevPath, prevAddr, prevKube })
	dbPath = filepath.Join(t.TempDir(), "nested", "traces.db")
	ingestAddr = "127.0.0.1:0"
	newKubeClients = func() (dynamic.Interface, kubernetes.Interface, error) {
		return newFakeDynamic(), kubefake.NewSimpleClientset(), nil
	}

	client, ingest, addr, closeStore, err := New("ns-default")
	if err != nil {
		t.Fatal(err)
	}
	if client == nil || ingest == nil || addr != "127.0.0.1:0" {
		t.Fatalf("client=%v ingest=%v addr=%q", client, ingest, addr)
	}
	if client.NamespaceFor("x") != "ns-default" {
		t.Fatalf("namespace = %q", client.NamespaceFor("x"))
	}
	if err := closeStore(); err != nil {
		t.Fatal(err)
	}
}

func TestNewClosesStoreWhenKubeClientsFail(t *testing.T) {
	prevPath, prevKube := dbPath, newKubeClients
	t.Cleanup(func() { dbPath, newKubeClients = prevPath, prevKube })
	dbPath = filepath.Join(t.TempDir(), "traces.db")
	wantErr := errors.New("no kubeconfig")
	newKubeClients = func() (dynamic.Interface, kubernetes.Interface, error) { return nil, nil, wantErr }

	if _, _, _, _, err := New("ns"); !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}
