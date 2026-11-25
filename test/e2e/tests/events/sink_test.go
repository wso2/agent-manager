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

package events

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/gomega"
)

// delivery is one request the sink received.
type delivery struct {
	ID, Timestamp, Signature string
	Body                     []byte
	At                       time.Time
}

// sink is a webhook receiver in the test process. It answers 503 to the
// next deliveries that carry failFor, then 200, so a spec can make the
// platform retry. Matching on content keeps another spec's event, which the
// same subscription can receive meanwhile, from taking the failure.
type sink struct {
	mu       sync.Mutex
	got      []delivery
	failFor  string
	failNext int
	srv      *http.Server
	URL      string
}

// sinkHost is the name amp-api reaches this process by: E2E_SINK_HOST, else
// host.docker.internal (the stack's amp-api needs that host mapped).
func sinkHost() string {
	if h := os.Getenv("E2E_SINK_HOST"); h != "" {
		return h
	}
	return "host.docker.internal"
}

func startSink() *sink {
	l, err := net.Listen("tcp", ":0")
	Expect(err).NotTo(HaveOccurred())
	s := &sink{}
	s.srv = &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.got = append(s.got, delivery{
			ID: r.Header.Get("webhook-id"), Timestamp: r.Header.Get("webhook-timestamp"),
			Signature: r.Header.Get("webhook-signature"), Body: body, At: time.Now(),
		})
		status := http.StatusOK
		if s.failNext > 0 && strings.Contains(string(body), s.failFor) {
			s.failNext--
			status = http.StatusServiceUnavailable
		}
		s.mu.Unlock()
		w.WriteHeader(status)
	})}
	go func() { _ = s.srv.Serve(l) }()
	s.URL = "http://" + sinkHost() + ":" + strconv.Itoa(l.Addr().(*net.TCPAddr).Port) + "/hook"
	return s
}

func (s *sink) stop() { _ = s.srv.Close() }

// failNextN fails the next n deliveries whose body contains needle.
func (s *sink) failNextN(needle string, n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failFor, s.failNext = needle, n
}

// matching returns the deliveries whose body contains needle.
func (s *sink) matching(needle string) []delivery {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []delivery
	for _, d := range s.got {
		if strings.Contains(string(d.Body), needle) {
			out = append(out, d)
		}
	}
	return out
}

// verifySignature checks a Standard Webhooks signature against a whsec_ secret.
func verifySignature(secret string, d delivery) bool {
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, "whsec_"))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(d.ID + "." + d.Timestamp + "." + string(d.Body)))
	want := "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
	for _, sig := range strings.Fields(d.Signature) {
		if hmac.Equal([]byte(sig), []byte(want)) {
			return true
		}
	}
	return false
}
