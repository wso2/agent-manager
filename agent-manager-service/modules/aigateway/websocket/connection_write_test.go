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

package websocket

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// overlapTransport counts writes that start while another is in progress,
// as gorilla's connection forbids.
type overlapTransport struct {
	Transport
	writing  atomic.Int32
	overlaps atomic.Int32
}

func (t *overlapTransport) write() error {
	if t.writing.Add(1) > 1 {
		t.overlaps.Add(1)
	}
	time.Sleep(10 * time.Microsecond)
	t.writing.Add(-1)
	return nil
}

func (t *overlapTransport) Send([]byte) error { return t.write() }
func (t *overlapTransport) SendPing() error   { return t.write() }

// Sends and heartbeat pings on one connection never write at the same time.
func TestConnectionSerializesSendsAndPings(t *testing.T) {
	tr := &overlapTransport{}
	conn := NewConnection("gw", "conn", tr, "token")

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				assert.NoError(t, conn.Send([]byte("event")))
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				assert.NoError(t, conn.SendPing())
			}
		}()
	}
	wg.Wait()
	assert.Zero(t, tr.overlaps.Load(), "writes overlapped on one connection")
}
