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

import type { Environment, PromotionPath } from "@agent-management-platform/types";
import { Box, Chip, Tooltip, Typography } from "@wso2/oxygen-ui";
import { ArrowRight } from "@wso2/oxygen-ui-icons-react";

export function buildPromotionChain(paths: PromotionPath[]): string[] {
  if (!paths.length) return [];

  const allTargets = new Set(paths.flatMap((p) => p.targetEnvironmentRefs.map((t) => t.name)));
  const adjacency = new Map<string, string[]>(
    paths.map((p) => [p.sourceEnvironmentRef, p.targetEnvironmentRefs.map((t) => t.name)])
  );

  const roots = [...new Set(paths.map((p) => p.sourceEnvironmentRef))].filter(
    (s) => !allTargets.has(s)
  );

  const chain: string[] = [];
  const visited = new Set<string>();
  let current: string | undefined = roots[0];

  while (current && !visited.has(current)) {
    chain.push(current);
    visited.add(current);
    current = (adjacency.get(current) ?? [])[0];
  }

  allTargets.forEach((t) => {
    if (!visited.has(t)) chain.push(t);
  });

  return chain;
}

export function PromotionChainChips({
  chain,
  envMap,
}: {
  chain: string[];
  envMap: Map<string, Environment>;
}) {
  if (!chain.length) {
    return (
      <Typography variant="body2" color="text.disabled" sx={{ fontStyle: "italic" }}>
        No promotion paths defined.
      </Typography>
    );
  }

  return (
    <>
      {chain.map((envName, index) => {
        const env = envMap.get(envName);
        const label = env?.displayName || envName;
        const isProd = env?.isProduction ?? false;
        return (
          <Box key={envName} display="flex" alignItems="center" gap={0.75}>
            <Tooltip title={isProd ? "Production" : ""} disableHoverListener={!isProd}>
              <Chip
                label={label}
                size="small"
                color={isProd ? "primary" : "default"}
                variant="outlined"
              />
            </Tooltip>
            {index < chain.length - 1 && (
              <ArrowRight size={14} style={{ opacity: 0.45, flexShrink: 0 }} />
            )}
          </Box>
        );
      })}
    </>
  );
}
