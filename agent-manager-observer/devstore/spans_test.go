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
	"path/filepath"
	"sync"
	"testing"
	"time"
)

const (
	testNS   = "default"
	testComp = "comp-1"
	testEnv  = "env-1"
)

func openTestStore(t *testing.T) *SpanStore {
	t.Helper()
	s, err := OpenSpanStore(filepath.Join(t.TempDir(), "nested", "traces.db"))
	if err != nil {
		t.Fatalf("OpenSpanStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func mkSpan(trace, id, parent string, start, end int64) Span {
	return Span{
		TraceID: trace, SpanID: id, ParentSpanID: parent, Name: "span-" + id, Kind: "Internal",
		StatusCode: "ok", StartNs: start, EndNs: end,
		Namespace: testNS, ProjectUID: "proj-1", ComponentUID: testComp, EnvironmentUID: testEnv,
	}
}

func mustInsert(t *testing.T, s *SpanStore, spans ...Span) {
	t.Helper()
	if err := s.Insert(context.Background(), spans); err != nil {
		t.Fatalf("Insert: %v", err)
	}
}

func allWindow() Window { return Window{StartNs: 0, EndNs: 1 << 60, Limit: 100, Desc: true} }

func TestQueryTracesListsRootedTracesOnly(t *testing.T) {
	s := openTestStore(t)
	mustInsert(t, s,
		mkSpan("rooted", "r", "", 100, 900),
		mkSpan("rooted", "c", "r", 200, 300),
		mkSpan("orphan", "c1", "missing-parent", 100, 200),
	)
	traces, total, err := s.QueryTraces(context.Background(), Scope{Namespace: testNS}, allWindow())
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(traces) != 1 || traces[0].TraceID != "rooted" {
		t.Fatalf("got total=%d traces=%+v", total, traces)
	}
}

func TestQueryTracesWindowRequiresContainment(t *testing.T) {
	s := openTestStore(t)
	mustInsert(t, s,
		mkSpan("late-root", "r", "", 100, 1500),
		mkSpan("late-root", "c", "r", 200, 300),
		mkSpan("ok", "r2", "", 100, 900),
		mkSpan("ok", "c2", "r2", 200, 300),
		mkSpan("ok", "late-child", "r2", 400, 1500),
	)
	w := Window{StartNs: 50, EndNs: 1000, Limit: 10, Desc: true}
	traces, total, err := s.QueryTraces(context.Background(), Scope{}, w)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(traces) != 1 || traces[0].TraceID != "ok" {
		t.Fatalf("got total=%d traces=%+v", total, traces)
	}
	if traces[0].SpanCount != 2 {
		t.Errorf("SpanCount = %d, want 2", traces[0].SpanCount)
	}
	if got := traces[0].EndTime; !got.Equal(time.Unix(0, 900)) {
		t.Errorf("EndTime = %v, want 900ns", got)
	}
}

func TestQueryTracesSummaryFields(t *testing.T) {
	s := openTestStore(t)
	root := mkSpan("t1", "root", "", 100, 900)
	root.Name, root.Kind = "invoke_agent", "Server"
	mustInsert(t, s, root, mkSpan("t1", "a", "root", 150, 400), mkSpan("t1", "b", "root", 200, 500))
	traces, _, err := s.QueryTraces(context.Background(), Scope{}, allWindow())
	if err != nil || len(traces) != 1 {
		t.Fatalf("err=%v traces=%+v", err, traces)
	}
	tr := traces[0]
	if tr.TraceName != "invoke_agent" || tr.RootSpanName != "invoke_agent" || tr.RootSpanID != "root" || tr.RootSpanKind != "Server" {
		t.Errorf("root fields: %+v", tr)
	}
	if tr.SpanCount != 3 || tr.DurationNs != 800 {
		t.Errorf("SpanCount=%d DurationNs=%d", tr.SpanCount, tr.DurationNs)
	}
	if tr.StartTime.Location() != time.UTC || tr.EndTime.Location() != time.UTC {
		t.Errorf("times not UTC: %v %v", tr.StartTime.Location(), tr.EndTime.Location())
	}
	if !tr.StartTime.Equal(time.Unix(0, 100)) {
		t.Errorf("StartTime = %v", tr.StartTime)
	}
}

func TestQueryTracesScopeFilter(t *testing.T) {
	s := openTestStore(t)
	other := mkSpan("other", "r", "", 100, 200)
	other.ComponentUID = "comp-2"
	otherNS := mkSpan("otherns", "r", "", 100, 200)
	otherNS.Namespace = "elsewhere"
	mustInsert(t, s, mkSpan("mine", "r", "", 100, 200), other, otherNS)

	cases := []struct {
		name  string
		scope Scope
		want  int
	}{
		{"no filter", Scope{}, 3},
		{"component", Scope{ComponentUID: testComp}, 2},
		{"component and namespace", Scope{Namespace: testNS, ComponentUID: testComp}, 1},
		{"environment", Scope{EnvironmentUID: testEnv}, 3},
		{"project", Scope{ProjectUID: "nope"}, 0},
	}
	for _, tc := range cases {
		_, total, err := s.QueryTraces(context.Background(), tc.scope, allWindow())
		if err != nil || total != tc.want {
			t.Errorf("%s: total=%d err=%v, want %d", tc.name, total, err, tc.want)
		}
	}
}

func TestQueryTracesOrderAndLimit(t *testing.T) {
	s := openTestStore(t)
	mustInsert(t, s,
		mkSpan("old", "r", "", 100, 200),
		mkSpan("mid", "r", "", 300, 400),
		mkSpan("new", "r", "", 500, 600),
	)
	w := Window{StartNs: 0, EndNs: 1000, Limit: 1, Desc: true}
	traces, total, err := s.QueryTraces(context.Background(), Scope{}, w)
	if err != nil || total != 3 || len(traces) != 1 || traces[0].TraceID != "new" {
		t.Fatalf("desc: total=%d traces=%+v err=%v", total, traces, err)
	}
	w.Desc = false
	traces, _, err = s.QueryTraces(context.Background(), Scope{}, w)
	if err != nil || len(traces) != 1 || traces[0].TraceID != "old" {
		t.Fatalf("asc: traces=%+v err=%v", traces, err)
	}
}

func TestTraceSpansScopeWindowLimitTotal(t *testing.T) {
	s := openTestStore(t)
	spans := []Span{mkSpan("t", "s0", "", 100, 900)}
	for i := 1; i < 5; i++ {
		spans = append(spans, mkSpan("t", fmt.Sprintf("s%d", i), "s0", int64(100+i*10), int64(200+i*10)))
	}
	mustInsert(t, s, spans...)

	got, total, err := s.TraceSpans(context.Background(), "t", Scope{ComponentUID: testComp},
		Window{StartNs: 0, EndNs: 1000, Limit: 2, Desc: true}, false)
	if err != nil || total != 5 || len(got) != 2 {
		t.Fatalf("total=%d len=%d err=%v", total, len(got), err)
	}
	if got[0].SpanID != "s4" || got[1].SpanID != "s3" {
		t.Errorf("order: %s, %s", got[0].SpanID, got[1].SpanID)
	}
	got, total, err = s.TraceSpans(context.Background(), "t", Scope{ComponentUID: "other"}, allWindow(), false)
	if err != nil || total != 0 || len(got) != 0 {
		t.Fatalf("other component: total=%d len=%d err=%v", total, len(got), err)
	}
}

func TestTraceSpansAttributesOnlyWhenRequested(t *testing.T) {
	s := openTestStore(t)
	sp := mkSpan("t", "s", "", 100, 200)
	sp.Attributes = map[string]any{"gen_ai.usage.input_tokens": int64(12), "name": "x"}
	sp.ResourceAttributes = map[string]string{"openchoreo.dev/component-uid": testComp}
	mustInsert(t, s, sp)

	without, _, err := s.TraceSpans(context.Background(), "t", Scope{}, allWindow(), false)
	if err != nil || len(without) != 1 {
		t.Fatalf("err=%v spans=%+v", err, without)
	}
	if without[0].Attributes != nil || without[0].ResourceAttributes != nil {
		t.Errorf("attributes present without request: %+v", without[0])
	}
	with, _, err := s.TraceSpans(context.Background(), "t", Scope{}, allWindow(), true)
	if err != nil || len(with) != 1 {
		t.Fatalf("err=%v spans=%+v", err, with)
	}
	if v, ok := with[0].Attributes["gen_ai.usage.input_tokens"].(float64); !ok || v != 12 {
		t.Errorf("attribute = %#v", with[0].Attributes["gen_ai.usage.input_tokens"])
	}
	if v, ok := with[0].ResourceAttributes["openchoreo.dev/component-uid"].(string); !ok || v != testComp {
		t.Errorf("resource attribute = %#v", with[0].ResourceAttributes)
	}
}

func TestStatusAlwaysNonNil(t *testing.T) {
	s := openTestStore(t)
	unset := mkSpan("t", "unset", "", 100, 200)
	unset.StatusCode = ""
	failed := mkSpan("t", "failed", "unset", 110, 190)
	failed.StatusCode, failed.StatusMessage = "error", "boom"
	mustInsert(t, s, unset, failed)

	spans, _, err := s.TraceSpans(context.Background(), "t", Scope{}, Window{EndNs: 1000, Limit: 10}, false)
	if err != nil || len(spans) != 2 {
		t.Fatalf("err=%v spans=%+v", err, spans)
	}
	for _, sp := range spans {
		if sp.Status == nil {
			t.Fatalf("span %s has nil status", sp.SpanID)
		}
		want := map[string]string{"unset": "unset", "failed": "error"}[sp.SpanID]
		if sp.Status.Code != want {
			t.Errorf("span %s status = %q, want %q", sp.SpanID, sp.Status.Code, want)
		}
	}
	d, err := s.SpanDetails(context.Background(), "t", "failed")
	if err != nil || d.Status == nil || d.Status.Code != "error" || d.Status.Message != "boom" {
		t.Fatalf("details: %+v err=%v", d, err)
	}
	d, err = s.SpanDetails(context.Background(), "t", "unset")
	if err != nil || d.Status == nil || d.Status.Code != "unset" {
		t.Fatalf("details: %+v err=%v", d, err)
	}
}

func TestInsertSpansIsIdempotent(t *testing.T) {
	s := openTestStore(t)
	batch := []Span{mkSpan("t", "r", "", 100, 200), mkSpan("t", "c", "r", 120, 180)}
	mustInsert(t, s, batch...)
	mustInsert(t, s, batch...)
	_, total, err := s.TraceSpans(context.Background(), "t", Scope{}, allWindow(), false)
	if err != nil || total != 2 {
		t.Fatalf("total=%d err=%v", total, err)
	}
}

func TestInsertIsAtomic(t *testing.T) {
	s := openTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	batch := make([]Span, 200)
	for i := range batch {
		batch[i] = mkSpan("t", fmt.Sprintf("s%d", i), "", int64(i), int64(i+1))
	}
	batch[100].Attributes = map[string]any{"cancel": cancelOnMarshal{cancel}}
	if err := s.Insert(ctx, batch); err == nil {
		t.Fatal("expected an error from the cancelled context")
	}
	_, total, err := s.TraceSpans(context.Background(), "t", Scope{}, allWindow(), false)
	if err != nil || total != 0 {
		t.Fatalf("partial batch stored: total=%d err=%v", total, err)
	}
}

type cancelOnMarshal struct{ cancel context.CancelFunc }

func (c cancelOnMarshal) MarshalJSON() ([]byte, error) {
	c.cancel()
	return []byte(`"x"`), nil
}

func TestSpanDetailsNotFound(t *testing.T) {
	s := openTestStore(t)
	resp, err := s.SpanDetails(context.Background(), "nope", "nope")
	if !errors.Is(err, ErrSpanNotFound) || resp != nil {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
}

func TestSpanDetailsReturnsFields(t *testing.T) {
	s := openTestStore(t)
	sp := mkSpan("t", "s", "p", 100, 350)
	sp.Kind = "Client"
	sp.Attributes = map[string]any{"k": "v"}
	sp.ResourceAttributes = map[string]string{"r": "1"}
	mustInsert(t, s, sp)
	d, err := s.SpanDetails(context.Background(), "t", "s")
	if err != nil {
		t.Fatal(err)
	}
	if d.SpanName != "span-s" || d.ParentSpanID != "p" || d.Kind != "Client" || d.DurationNs != 250 ||
		d.Attributes["k"] != "v" || d.ResourceAttributes["r"] != "1" || d.StartTime.Location() != time.UTC {
		t.Errorf("details: %+v", d)
	}
}

func TestConcurrentInsertAndQuery(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	deadline := time.Now().Add(2 * time.Second)
	errs := make(chan error, 8)
	var wg sync.WaitGroup
	run := func(f func(i int) error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; time.Now().Before(deadline); i++ {
				if err := f(i); err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	run(func(i int) error {
		batch := make([]Span, 512)
		for j := range batch {
			parent := "root"
			if j == 0 {
				parent = ""
			}
			batch[j] = mkSpan(fmt.Sprintf("t%d", i%5), fmt.Sprintf("s%d-%d", i, j), parent, int64(j), int64(j+10))
			if j == 0 {
				batch[j].SpanID = "root"
			}
		}
		return s.Insert(ctx, batch)
	})
	run(func(int) error { _, _, err := s.QueryTraces(ctx, Scope{}, allWindow()); return err })
	run(func(int) error { _, _, err := s.TraceSpans(ctx, "t0", Scope{}, allWindow(), true); return err })
	run(func(int) error {
		_, err := s.SpanDetails(ctx, "t0", "root")
		if errors.Is(err, ErrSpanNotFound) {
			return nil
		}
		return err
	})

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("hang")
	}
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
