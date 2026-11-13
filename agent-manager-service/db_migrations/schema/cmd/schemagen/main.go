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

// Command schemagen renders db_migrations/schema/lite.yaml into the per-database baseline migrations. Run it from the
// module root (`make schema`).
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/wso2/agent-manager/agent-manager-service/db_migrations/schema"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "schemagen:", err)
		os.Exit(1)
	}
}

func run() error {
	s, err := schema.Lite()
	if err != nil {
		return err
	}
	for _, dialect := range schema.Dialects {
		ddl, err := s.Render(dialect)
		if err != nil {
			return err
		}
		path := schema.BaselinePath(dialect)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(ddl), 0o600); err != nil {
			return err
		}
		fmt.Println("wrote", path)
	}
	return nil
}
