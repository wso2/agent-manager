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

package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/internal/modhost"
)

func TestPrintMigrations(t *testing.T) {
	var b strings.Builder
	printMigrations(&b, []modhost.MigrationResult{
		{Module: "ai-gateway", Applied: 0, Total: 3},
		{Module: "observability", External: &modhost.ExternalResult{Store: "clickhouse", Applied: 1, Total: 1}},
		{Module: "other", External: &modhost.ExternalResult{Skipped: "no store address"}},
		{Module: "quiet"},
	})
	want := "ai-gateway: applied 0 of 3 migration(s)\n" +
		"observability: applied 1 of 1 clickhouse schema file(s)\n" +
		"other: skipped its external schema: no store address\n"
	if b.String() != want {
		t.Errorf("printMigrations wrote\n%s\nwant\n%s", b.String(), want)
	}
}

// fakeModuleSchema has current core schemas and returns external's errors in turn.
type fakeModuleSchema struct {
	external []error
	calls    int
}

func (*fakeModuleSchema) CheckSchema(context.Context) error { return nil }

func (f *fakeModuleSchema) CheckExternalSchema(context.Context) error {
	err := f.external[min(f.calls, len(f.external)-1)]
	f.calls++
	return err
}

// A config error fails the wait at once; pending files are waited for.
func TestWaitForModuleSchemaExternalErrors(t *testing.T) {
	configErr := errors.New("module observability external schema: clickhouse.url is set but clickhouse.user is not")
	f := &fakeModuleSchema{external: []error{configErr}}
	if err := waitForModuleSchema(context.Background(), f); !errors.Is(err, configErr) || f.calls != 1 {
		t.Errorf("config error: waitForModuleSchema = %v after %d checks, want it at once", err, f.calls)
	}

	behind := fmt.Errorf("%w (observability: 1 clickhouse file(s))", modhost.ErrExternalBehind)
	f = &fakeModuleSchema{external: []error{behind}}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := waitForModuleSchema(ctx, f); !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, modhost.ErrExternalBehind) {
		t.Errorf("pending files: waitForModuleSchema = %v, want it to wait until the deadline", err)
	}
}
