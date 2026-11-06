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
package store

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
)

const (
	runtimeContainer      = "main"
	workflowNamespacePref = "workflows-"
	workflowLabel         = "workflows.argoproj.io/workflow"
)

var workflowSkippedContainers = []string{"init", "wait"}

type logOpener func(ctx context.Context, ns, pod string, opts *corev1.PodLogOptions) (io.ReadCloser, error)

// LogReader serves log queries straight from pod logs in the Kubernetes API.
type LogReader struct {
	cs   kubernetes.Interface
	open logOpener
}

func NewLogReader(cs kubernetes.Interface) *LogReader {
	return &LogReader{
		cs: cs,
		open: func(ctx context.Context, ns, pod string, opts *corev1.PodLogOptions) (io.ReadCloser, error) {
			return cs.CoreV1().Pods(ns).GetLogs(pod, opts).Stream(ctx)
		},
	}
}

type logTarget struct {
	namespace, pod, container string
	previous                  bool
}

type logSelection struct {
	listNamespace string
	selector      string
	targets       func(pod *corev1.Pod) []logTarget
	withLevel     bool
}

// Query ignores req.Limit and req.SortOrder; the caller passes the effective values.
func (l *LogReader) Query(ctx context.Context, req observer.LogsQueryRequest, limit int, desc bool) (*observer.LogsQueryResponse, error) {
	sel, err := selectionFor(req.SearchScope)
	if err != nil {
		return nil, err
	}
	pods, err := l.cs.CoreV1().Pods(sel.listNamespace).List(ctx, metav1.ListOptions{LabelSelector: sel.selector})
	if err != nil {
		return nil, fmt.Errorf("list pods: %w", err)
	}

	filter := newLineFilter(req, sel.withLevel)
	var entries []observer.LogEntry
	for i := range pods.Items {
		for _, target := range sel.targets(&pods.Items[i]) {
			stopAfter := limit
			if desc {
				stopAfter = 0
			}
			got, err := l.readTarget(ctx, target, req.StartTime, filter, stopAfter)
			if err != nil {
				slog.Warn("skip pod logs", "namespace", target.namespace, "pod", target.pod,
					"container", target.container, "previous", target.previous, "error", err)
				continue
			}
			entries = append(entries, got...)
		}
	}

	slices.SortStableFunc(entries, func(a, b observer.LogEntry) int {
		if desc {
			return b.Timestamp.Compare(*a.Timestamp)
		}
		return a.Timestamp.Compare(*b.Timestamp)
	})
	if len(entries) > limit {
		entries = entries[:limit]
	}
	if entries == nil {
		entries = []observer.LogEntry{}
	}
	return &observer.LogsQueryResponse{Logs: entries}, nil
}

func selectionFor(scope any) (logSelection, error) {
	switch s := scope.(type) {
	case observer.ComponentSearchScope:
		return runtimeSelection(s), nil
	case *observer.ComponentSearchScope:
		if s != nil {
			return runtimeSelection(*s), nil
		}
	case observer.WorkflowSearchScope:
		return buildSelection(s), nil
	case *observer.WorkflowSearchScope:
		if s != nil {
			return buildSelection(*s), nil
		}
	}
	return logSelection{}, errors.New("unsupported search scope for dev logs")
}

func runtimeSelection(scope observer.ComponentSearchScope) logSelection {
	labels := []string{"openchoreo.dev/namespace=" + scope.Namespace}
	for key, value := range map[string]*string{
		"openchoreo.dev/project":     scope.Project,
		"openchoreo.dev/component":   scope.Component,
		"openchoreo.dev/environment": scope.Environment,
	} {
		if value != nil {
			labels = append(labels, key+"="+*value)
		}
	}
	slices.Sort(labels)
	return logSelection{
		listNamespace: metav1.NamespaceAll,
		selector:      strings.Join(labels, ","),
		withLevel:     true,
		targets: func(pod *corev1.Pod) []logTarget {
			current := logTarget{namespace: pod.Namespace, pod: pod.Name, container: runtimeContainer}
			if !hasRestarted(pod, runtimeContainer) {
				return []logTarget{current}
			}
			previous := current
			previous.previous = true
			return []logTarget{current, previous}
		},
	}
}

func hasRestarted(pod *corev1.Pod, container string) bool {
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name == container {
			return status.RestartCount > 0
		}
	}
	return false
}

func buildSelection(scope observer.WorkflowSearchScope) logSelection {
	selector := ""
	if scope.WorkflowRunName != nil {
		selector = workflowLabel + "=" + *scope.WorkflowRunName
	}
	return logSelection{
		listNamespace: workflowNamespacePref + scope.Namespace,
		selector:      selector,
		targets: func(pod *corev1.Pod) []logTarget {
			var targets []logTarget
			for _, c := range pod.Spec.Containers {
				if !slices.Contains(workflowSkippedContainers, c.Name) {
					targets = append(targets, logTarget{namespace: pod.Namespace, pod: pod.Name, container: c.Name})
				}
			}
			return targets
		},
	}
}

type lineFilter struct {
	start, end time.Time
	phrase     string
	levels     []string
	withLevel  bool
	needLevel  bool
}

func newLineFilter(req observer.LogsQueryRequest, componentScope bool) lineFilter {
	f := lineFilter{start: req.StartTime, end: req.EndTime, withLevel: componentScope}
	if req.SearchPhrase != nil {
		f.phrase = strings.ToLower(*req.SearchPhrase)
	}
	for _, level := range req.LogLevels {
		f.levels = append(f.levels, strings.ToUpper(level))
	}
	f.needLevel = componentScope || len(f.levels) > 0
	return f
}

// Levels are matched exactly against the extracted level, which is stricter than the
// OpenSearch adapter's substring match.
func (f lineFilter) accept(line string) (observer.LogEntry, bool) {
	stamp, text, ok := strings.Cut(strings.TrimRight(line, "\r\n"), " ")
	if !ok {
		return observer.LogEntry{}, false
	}
	ts, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil || ts.Before(f.start) || ts.After(f.end) {
		return observer.LogEntry{}, false
	}
	if f.phrase != "" && !strings.Contains(strings.ToLower(text), f.phrase) {
		return observer.LogEntry{}, false
	}
	entry := observer.LogEntry{Timestamp: &ts, Log: &text}
	if !f.needLevel {
		return entry, true
	}
	level := extractLogLevel(text)
	if len(f.levels) > 0 && !slices.Contains(f.levels, level) {
		return observer.LogEntry{}, false
	}
	if f.withLevel {
		entry.Level = &level
	}
	return entry, true
}

// readTarget stops after stopAfter accepted lines when stopAfter is positive; streams are
// time-ordered, so later lines cannot make the merged first stopAfter.
func (l *LogReader) readTarget(ctx context.Context, target logTarget, start time.Time, filter lineFilter, stopAfter int) ([]observer.LogEntry, error) {
	sinceTime := metav1.NewTime(start.Truncate(time.Second))
	stream, err := l.open(ctx, target.namespace, target.pod, &corev1.PodLogOptions{
		Container:  target.container,
		Previous:   target.previous,
		Timestamps: true,
		SinceTime:  &sinceTime,
	})
	if err != nil {
		return nil, fmt.Errorf("open logs: %w", err)
	}
	defer func() { _ = stream.Close() }()

	var entries []observer.LogEntry
	reader := bufio.NewReader(stream)
	for stopAfter <= 0 || len(entries) < stopAfter {
		line, readErr := reader.ReadString('\n')
		if entry, ok := filter.accept(line); ok {
			entries = append(entries, entry)
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return entries, fmt.Errorf("read logs: %w", readErr)
		}
	}
	return entries, nil
}
