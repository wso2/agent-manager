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
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	embeddedStartTimeout = 10 * time.Second
	// embeddedMaxMemory bounds what JetStream may hold in memory, across the
	// events and deliveries streams.
	embeddedMaxMemory = 256 << 20
)

// StartEmbedded runs a NATS server inside this process, with JetStream kept
// in memory, and connects to it in-process. Nothing listens on the network.
//
// It is meant for quick-start and single-replica installs: queued events and
// pending retries are lost when the process restarts, and every replica would
// get a server of its own. Production installs connect to a NATS server.
func StartEmbedded(logger *slog.Logger) (*NATSBus, error) {
	// JetStream requires a store directory even when every stream is kept in
	// memory; nothing is written to it.
	storeDir, err := os.MkdirTemp("", "amp-nats-")
	if err != nil {
		return nil, fmt.Errorf("failed to create embedded NATS store directory: %w", err)
	}
	srv, err := server.NewServer(&server.Options{
		ServerName:         "amp-embedded",
		DontListen:         true,
		JetStream:          true,
		JetStreamMaxMemory: embeddedMaxMemory,
		JetStreamMaxStore:  0,
		StoreDir:           storeDir,
		NoSigs:             true,
	})
	if err != nil {
		_ = os.RemoveAll(storeDir)
		return nil, fmt.Errorf("failed to create embedded NATS server: %w", err)
	}
	go srv.Start()
	if !srv.ReadyForConnections(embeddedStartTimeout) {
		srv.Shutdown()
		_ = os.RemoveAll(storeDir)
		return nil, errors.New("embedded NATS server did not start")
	}

	nc, err := nats.Connect("", nats.Name("agent-manager-service"), nats.InProcessServer(srv))
	if err != nil {
		srv.Shutdown()
		_ = os.RemoveAll(storeDir)
		return nil, fmt.Errorf("failed to connect to embedded NATS server: %w", err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		srv.Shutdown()
		_ = os.RemoveAll(storeDir)
		return nil, fmt.Errorf("failed to open JetStream: %w", err)
	}
	logger.Info("Events use an embedded in-memory NATS server; queued events do not survive a restart")
	return &NATSBus{nc: nc, js: js, storage: jetstream.MemoryStorage, embedded: srv, storeDir: storeDir}, nil
}
