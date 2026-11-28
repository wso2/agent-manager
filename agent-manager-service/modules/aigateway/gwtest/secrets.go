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

package gwtest

import (
	"context"
	"errors"
	"sync"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
)

// Secrets is an in-memory modkit.Secrets for tests, keyed by org and path.
// It ignores transactions: a rolled-back write stays.
type Secrets struct {
	mu sync.Mutex
	m  map[string]string
}

var _ modkit.Secrets = (*Secrets)(nil)

// NewSecrets returns an empty store.
func NewSecrets() *Secrets { return &Secrets{m: map[string]string{}} }

func (s *Secrets) Put(_ context.Context, orgID, path, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[orgID+"|"+path] = value
	return nil
}

func (s *Secrets) Get(_ context.Context, orgID, path string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[orgID+"|"+path]
	if !ok {
		return "", modkit.ErrNotFound
	}
	return v, nil
}

func (s *Secrets) Delete(_ context.Context, orgID, path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, orgID+"|"+path)
	return nil
}

// Create stores value at path unless path holds one, and returns what path holds.
func (s *Secrets) Create(_ context.Context, orgID, path, value string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.m[orgID+"|"+path]; ok {
		return v, nil
	}
	s.m[orgID+"|"+path] = value
	return value, nil
}

func (s *Secrets) In(*gorm.DB) modkit.Secrets { return s }

// Prepare has nothing to stage: the fake keeps values in memory.
func (s *Secrets) Prepare(ctx context.Context, _, _, _ string) (context.Context, error) {
	return ctx, nil
}

// Len is how many values the store holds.
func (s *Secrets) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.m)
}

// StagingSecrets behaves like an external vault: Prepare refuses empty values
// and Put refuses any value that was not staged first.
type StagingSecrets struct {
	*Secrets
	staged map[string]string
}

// NewStagingSecrets returns an empty staging store.
func NewStagingSecrets() *StagingSecrets {
	return &StagingSecrets{Secrets: NewSecrets(), staged: map[string]string{}}
}

func (s *StagingSecrets) Prepare(ctx context.Context, orgID, path, value string) (context.Context, error) {
	if value == "" {
		return ctx, errors.New("secret value cannot be empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.staged[orgID+"|"+path] = value
	return ctx, nil
}

func (s *StagingSecrets) Put(ctx context.Context, orgID, path, value string) error {
	s.mu.Lock()
	staged, ok := s.staged[orgID+"|"+path]
	s.mu.Unlock()
	if !ok || staged != value {
		return errors.New("secret value was not staged")
	}
	return s.Secrets.Put(ctx, orgID, path, value)
}

func (s *StagingSecrets) In(*gorm.DB) modkit.Secrets { return s }
