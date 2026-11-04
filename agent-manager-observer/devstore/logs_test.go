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
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
)

type openCall struct {
	ns, pod string
	opts    corev1.PodLogOptions
}

type fakeLogs struct {
	mu     sync.Mutex
	calls  []openCall
	lines  map[string]string
	errors map[string]error
}

func (f *fakeLogs) open(_ context.Context, ns, pod string, opts *corev1.PodLogOptions) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, openCall{ns: ns, pod: pod, opts: *opts})
	key := logKey(ns, pod, opts.Container, opts.Previous)
	if err := f.errors[key]; err != nil {
		return nil, err
	}
	return io.NopCloser(strings.NewReader(f.lines[key])), nil
}

func logKey(ns, pod, container string, previous bool) string {
	return fmt.Sprintf("%s/%s/%s/%t", ns, pod, container, previous)
}

func (f *fakeLogs) containersRead() []string {
	var out []string
	for _, c := range f.calls {
		out = append(out, logKey(c.ns, c.pod, c.opts.Container, c.opts.Previous))
	}
	return out
}

func newPod(ns, name string, labels map[string]string, restarts int32, containers ...string) *corev1.Pod {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name, Labels: labels}}
	for _, c := range containers {
		pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{Name: c})
		pod.Status.ContainerStatuses = append(pod.Status.ContainerStatuses,
			corev1.ContainerStatus{Name: c, RestartCount: restarts})
	}
	return pod
}

func runtimeLabels(project, component, env string) map[string]string {
	return map[string]string{
		"openchoreo.dev/namespace":   "default",
		"openchoreo.dev/project":     project,
		"openchoreo.dev/component":   component,
		"openchoreo.dev/environment": env,
	}
}

var (
	windowStart = time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	windowEnd   = windowStart.Add(time.Hour)
)

func stamped(offset time.Duration, text string) string {
	return windowStart.Add(offset).Format(time.RFC3339Nano) + " " + text + "\n"
}

func componentRequest() observer.LogsQueryRequest {
	project, component, env := "p", "c", "dev"
	return observer.LogsQueryRequest{
		StartTime: windowStart,
		EndTime:   windowEnd,
		SearchScope: observer.ComponentSearchScope{
			Namespace: "default", Project: &project, Component: &component, Environment: &env,
		},
	}
}

func newReader(t *testing.T, fl *fakeLogs, pods ...*corev1.Pod) *LogReader {
	t.Helper()
	cs := fake.NewClientset()
	for _, p := range pods {
		if _, err := cs.CoreV1().Pods(p.Namespace).Create(context.Background(), p, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	reader := NewLogReader(cs)
	reader.open = fl.open
	return reader
}

func TestRuntimeScopeSelectsPodsByNameLabels(t *testing.T) {
	fl := &fakeLogs{lines: map[string]string{
		"ns-a/match/main/false": stamped(time.Second, "hello"),
	}}
	reader := newReader(t, fl,
		newPod("ns-a", "match", runtimeLabels("p", "c", "dev"), 0, "main", "sidecar"),
		newPod("ns-b", "other-component", runtimeLabels("p", "other", "dev"), 0, "main"),
		newPod("ns-c", "other-env", runtimeLabels("p", "c", "prod"), 0, "main"),
	)

	resp, err := reader.Query(context.Background(), componentRequest(), 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := fl.containersRead(); len(got) != 1 || got[0] != "ns-a/match/main/false" {
		t.Fatalf("containers read = %v", got)
	}
	if len(resp.Logs) != 1 || *resp.Logs[0].Log != "hello" || *resp.Logs[0].Level != "INFO" {
		t.Fatalf("logs = %+v", resp.Logs)
	}
	if resp.Total != nil {
		t.Errorf("Total = %v, want nil", *resp.Total)
	}
}

func TestRuntimeReadsPreviousForRestartedPods(t *testing.T) {
	fl := &fakeLogs{}
	reader := newReader(t, fl, newPod("ns", "crashy", runtimeLabels("p", "c", "dev"), 2, "main"))

	if _, err := reader.Query(context.Background(), componentRequest(), 10, false); err != nil {
		t.Fatal(err)
	}
	got := fl.containersRead()
	if len(got) != 2 || got[0] != "ns/crashy/main/false" || got[1] != "ns/crashy/main/true" {
		t.Fatalf("containers read = %v", got)
	}
	for _, c := range fl.calls {
		if !c.opts.Timestamps || !c.opts.SinceTime.Time.Equal(windowStart) {
			t.Errorf("opts = %+v", c.opts)
		}
	}
}

func TestBuildScopeSkipsInitAndWait(t *testing.T) {
	fl := &fakeLogs{lines: map[string]string{
		"workflows-default/run-pod/build/false": stamped(time.Second, "compiling"),
	}}
	reader := newReader(t, fl,
		newPod("workflows-default", "run-pod", map[string]string{"workflows.argoproj.io/workflow": "run-1"},
			0, "init", "wait", "build"),
		newPod("workflows-default", "other-run", map[string]string{"workflows.argoproj.io/workflow": "run-2"}, 0, "build"),
	)
	run := "run-1"
	req := observer.LogsQueryRequest{
		StartTime:   windowStart,
		EndTime:     windowEnd,
		SearchScope: observer.WorkflowSearchScope{Namespace: "default", WorkflowRunName: &run},
	}

	resp, err := reader.Query(context.Background(), req, 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := fl.containersRead(); len(got) != 1 || got[0] != "workflows-default/run-pod/build/false" {
		t.Fatalf("containers read = %v", got)
	}
	if len(resp.Logs) != 1 || resp.Logs[0].Level != nil {
		t.Fatalf("logs = %+v", resp.Logs)
	}
}

func TestPodLogErrorIsSkipped(t *testing.T) {
	fl := &fakeLogs{
		errors: map[string]error{"ns/bad/main/false": errors.New("boom")},
		lines:  map[string]string{"ns/good/main/false": stamped(time.Second, "fine")},
	}
	reader := newReader(t, fl,
		newPod("ns", "bad", runtimeLabels("p", "c", "dev"), 0, "main"),
		newPod("ns", "good", runtimeLabels("p", "c", "dev"), 0, "main"),
	)

	resp, err := reader.Query(context.Background(), componentRequest(), 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Logs) != 1 || *resp.Logs[0].Log != "fine" {
		t.Fatalf("logs = %+v", resp.Logs)
	}
}

func TestNoPodsReturnsEmpty(t *testing.T) {
	reader := newReader(t, &fakeLogs{})
	resp, err := reader.Query(context.Background(), componentRequest(), 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Logs) != 0 {
		t.Fatalf("logs = %+v", resp.Logs)
	}
}

func TestUnsupportedScopeIsAnError(t *testing.T) {
	reader := newReader(t, &fakeLogs{})
	req := observer.LogsQueryRequest{StartTime: windowStart, EndTime: windowEnd, SearchScope: "nope"}
	if _, err := reader.Query(context.Background(), req, 10, false); err == nil {
		t.Fatal("want error")
	}
}

type countingReader struct {
	io.Reader
	closed bool
}

func (c *countingReader) Close() error { c.closed = true; return nil }

func TestAscStopsReadingAfterLimit(t *testing.T) {
	var body strings.Builder
	for i := range 5000 {
		body.WriteString(stamped(time.Duration(i)*time.Second, fmt.Sprintf("line %d", i)))
	}
	counter := &countingReader{Reader: strings.NewReader(body.String())}
	reader := newReader(t, &fakeLogs{}, newPod("ns", "p", runtimeLabels("p", "c", "dev"), 0, "main"))
	reader.open = func(context.Context, string, string, *corev1.PodLogOptions) (io.ReadCloser, error) {
		return counter, nil
	}

	resp, err := reader.Query(context.Background(), componentRequest(), 3, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Logs) != 3 || *resp.Logs[2].Log != "line 2" {
		t.Fatalf("logs = %+v", resp.Logs)
	}
	if !counter.closed {
		t.Error("stream not closed")
	}
	if remaining := counter.Reader.(*strings.Reader).Len(); remaining < body.Len()/2 {
		t.Errorf("read too much: %d bytes left of %d", remaining, body.Len())
	}
}

func TestFiltersWindowPhraseLevelSortLimit(t *testing.T) {
	body := stamped(-500*time.Millisecond, "ERROR before window") +
		stamped(1*time.Second, "ERROR disk Full") +
		stamped(2*time.Second, "INFO disk full") +
		stamped(3*time.Second, "ERROR disk full again") +
		stamped(4*time.Second, "ERROR network") +
		stamped(2*time.Hour, "ERROR disk full after window") +
		"not a timestamp line\n"
	fl := &fakeLogs{lines: map[string]string{"ns/p/main/false": body}}
	reader := newReader(t, fl, newPod("ns", "p", runtimeLabels("p", "c", "dev"), 0, "main"))
	req := componentRequest()
	phrase := "DISK full"
	req.SearchPhrase = &phrase
	req.LogLevels = []string{"ERROR"}

	resp, err := reader.Query(context.Background(), req, 1, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Logs) != 1 {
		t.Fatalf("logs = %+v", resp.Logs)
	}
	got := resp.Logs[0]
	if *got.Log != "ERROR disk full again" || *got.Level != "ERROR" {
		t.Errorf("log = %q level = %q", *got.Log, *got.Level)
	}
	if !got.Timestamp.Equal(windowStart.Add(3 * time.Second)) {
		t.Errorf("timestamp = %v", got.Timestamp)
	}

	resp, err = reader.Query(context.Background(), req, 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Logs) != 2 || *resp.Logs[0].Log != "ERROR disk Full" {
		t.Fatalf("asc logs = %+v", resp.Logs)
	}
}
