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
	"compress/gzip"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
)

const DefaultMaxIngestBytes = 20 << 20

const ingestPath = "/v1/traces"

var scopeHeaderAttributes = []struct{ header, attribute string }{
	{"x-user-component", "openchoreo.dev/component-uid"},
	{"x-user-environment", "openchoreo.dev/environment-uid"},
	{"x-user-project", "openchoreo.dev/project-uid"},
	{"x-user-namespace", "openchoreo.dev/namespace"},
}

type ingestHandler struct {
	store    *SpanStore
	maxBytes int64
}

func NewIngestHandler(store *SpanStore, maxBytes int64) http.Handler {
	return &ingestHandler{store: store, maxBytes: maxBytes}
}

func (h *ingestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != ingestPath {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !isProtobuf(r.Header.Get("Content-Type")) {
		http.Error(w, "only application/x-protobuf is accepted", http.StatusUnsupportedMediaType)
		return
	}
	reader, err := h.bodyReader(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
		return
	}
	body, err := io.ReadAll(io.LimitReader(reader, h.maxBytes+1))
	if err != nil {
		h.rejectUnreadable(w, err)
		return
	}
	if int64(len(body)) > h.maxBytes {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}
	req := ptraceotlp.NewExportRequest()
	if err := req.UnmarshalProto(body); err != nil {
		http.Error(w, "invalid OTLP protobuf: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.store.Insert(r.Context(), spansFromTraces(req.Traces(), scopeAttributes(r.Header))); err != nil {
		slog.Error("store ingested spans", "error", err)
		http.Error(w, "failed to store spans", http.StatusInternalServerError)
		return
	}
	resp, err := ptraceotlp.NewExportResponse().MarshalProto()
	if err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-protobuf")
	_, _ = w.Write(resp)
}

func isProtobuf(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	return err == nil && mediaType == "application/x-protobuf"
}

func (h *ingestHandler) bodyReader(r *http.Request) (io.Reader, error) {
	raw := io.Reader(http.MaxBytesReader(nil, r.Body, h.maxBytes))
	switch strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Encoding"))) {
	case "", "identity":
		return raw, nil
	case "gzip":
		return &lazyGzip{raw: raw}, nil
	default:
		return nil, errors.New("only gzip or no content encoding is accepted")
	}
}

// lazyGzip defers opening the gzip stream so a corrupt header surfaces as a read error and maps to 400.
type lazyGzip struct {
	raw io.Reader
	zr  *gzip.Reader
}

func (l *lazyGzip) Read(p []byte) (int, error) {
	if l.zr == nil {
		zr, err := gzip.NewReader(l.raw)
		if err != nil {
			return 0, &decodeError{err}
		}
		l.zr = zr
	}
	n, err := l.zr.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		var tooLarge *http.MaxBytesError
		if !errors.As(err, &tooLarge) {
			err = &decodeError{err}
		}
	}
	return n, err
}

type decodeError struct{ err error }

func (e *decodeError) Error() string { return "invalid gzip body: " + e.err.Error() }
func (e *decodeError) Unwrap() error { return e.err }

func (h *ingestHandler) rejectUnreadable(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}
	http.Error(w, "cannot read request body: "+err.Error(), http.StatusBadRequest)
}

func scopeAttributes(header http.Header) map[string]string {
	attrs := map[string]string{}
	for _, m := range scopeHeaderAttributes {
		if v := header.Get(m.header); v != "" {
			attrs[m.attribute] = v
		}
	}
	return attrs
}

func spansFromTraces(traces ptrace.Traces, scope map[string]string) []Span {
	var spans []Span
	resourceSpans := traces.ResourceSpans()
	for i := 0; i < resourceSpans.Len(); i++ {
		rs := resourceSpans.At(i)
		resAttrs := resourceAttributes(rs.Resource(), scope)
		for j := 0; j < rs.ScopeSpans().Len(); j++ {
			ss := rs.ScopeSpans().At(j).Spans()
			for k := 0; k < ss.Len(); k++ {
				spans = append(spans, toSpan(ss.At(k), resAttrs))
			}
		}
	}
	return spans
}

func resourceAttributes(res pcommon.Resource, scope map[string]string) map[string]string {
	attrs := make(map[string]string, res.Attributes().Len()+len(scope))
	res.Attributes().Range(func(k string, v pcommon.Value) bool {
		attrs[k] = v.AsString()
		return true
	})
	for k, v := range scope {
		attrs[k] = v
	}
	return attrs
}

func toSpan(span ptrace.Span, resAttrs map[string]string) Span {
	return Span{
		TraceID:            span.TraceID().String(),
		SpanID:             span.SpanID().String(),
		ParentSpanID:       span.ParentSpanID().String(),
		Name:               span.Name(),
		Kind:               span.Kind().String(),
		StartNs:            int64(span.StartTimestamp()),
		EndNs:              int64(span.EndTimestamp()),
		StatusCode:         spanStatus(span.Status().Code()),
		StatusMessage:      span.Status().Message(),
		Attributes:         span.Attributes().AsRaw(),
		ResourceAttributes: resAttrs,
		Namespace:          resAttrs["openchoreo.dev/namespace"],
		ProjectUID:         resAttrs["openchoreo.dev/project-uid"],
		ComponentUID:       resAttrs["openchoreo.dev/component-uid"],
		EnvironmentUID:     resAttrs["openchoreo.dev/environment-uid"],
	}
}

func spanStatus(code ptrace.StatusCode) string {
	switch status := strings.ToLower(code.String()); status {
	case "ok", "error":
		return status
	default:
		return "unset"
	}
}
