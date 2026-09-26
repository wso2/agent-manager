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

package middleware

import "net/http"

// responseRecorder captures the response status so middleware can record the
// outcome of a request. The only other ResponseWriter wrapper in this service
// is middleware/growthanalytics' statusRecordingWriter, which records status
// for telemetry; this one is what makes status observable to the audit trail.
//
// It implements Unwrap so http.ResponseController continues to find Flush,
// Hijack and the rest on the underlying writer. Without that, wrapping a
// handler here would silently break streaming responses if any were added.
type responseRecorder struct {
	http.ResponseWriter
	status  int
	written bool
	bytes   int64
}

func newResponseRecorder(w http.ResponseWriter) *responseRecorder {
	return &responseRecorder{ResponseWriter: w}
}

func (r *responseRecorder) WriteHeader(code int) {
	if !r.written {
		r.status = code
		r.written = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if !r.written {
		// net/http implies 200 on the first write without an explicit header.
		r.status = http.StatusOK
		r.written = true
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += int64(n)
	return n, err
}

// Unwrap exposes the underlying writer to http.ResponseController.
func (r *responseRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// Status returns the response status, defaulting to 200 for a handler that
// returned without writing anything.
func (r *responseRecorder) Status() int {
	if !r.written {
		return http.StatusOK
	}
	return r.status
}

// setStatus records a status the handler never wrote itself. Used when a panic
// unwinds past the audit middleware and the response will become a 500.
//
// A committed status wins. A handler that wrote 201 and then panicked did
// create the resource, and the client received the 201 — recording 500 there
// would make the trail contradict both the client and reality.
func (r *responseRecorder) setStatus(code int) {
	if r.written {
		return
	}
	r.status = code
	r.written = true
}
