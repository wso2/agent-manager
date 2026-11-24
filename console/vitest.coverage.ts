/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { existsSync, readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";

// Coverage settings shared by every package's vitest.config.ts. Off by default;
// `pnpm test:coverage` turns it on with --coverage.

const REEXPORT = /export\s+(type\s+)?(\*(\s+as\s+\w+)?|\{[^}]*\})\s+from\s+["'][^"']+["'];?/g;
const COMMENT = /\/\*[\s\S]*?\*\/|\/\/.*$/gm;

// index.ts files that only re-export other modules. Barrels with code of
// their own (route tables, components) stay in the report.
function pureBarrels(root: string): string[] {
  const src = join(root, "src");
  if (!existsSync(src)) return [];
  return readdirSync(src, { recursive: true, encoding: "utf8" })
    .filter((file) => /(^|[\\/])index\.tsx?$/.test(file))
    .filter((file) => {
      const code = readFileSync(join(src, file), "utf8");
      return code.replace(COMMENT, "").replace(REEXPORT, "").trim() === "";
    })
    .map((file) => `src/${file.split("\\").join("/")}`);
}

export const coverage = {
  enabled: false,
  provider: "v8" as const,
  reporter: ["json-summary", "text-summary"],
  include: ["src/**/*.{ts,tsx}"],
  exclude: [
    "src/**/*.stories.*",
    "src/**/*.{test,spec}.*",
    "src/**/*.d.ts",
    "src/**/generated/**",
    "src/**/*.generated.*",
    "src/**/generated-*",
    ...pureBarrels(process.cwd()),
  ],
};
