/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import type { PromotionPath } from "@agent-management-platform/types";

export interface PromotionChainValidation {
  valid: boolean;
  error?: string;
  /** Ordered list of env names if the chain is valid, e.g. ["dev","stage","prod"] */
  chain?: string[];
}

/**
 * Validates that a set of promotion paths forms a single connected DAG with
 * one root (entry point). Paths like dev→stage and stage→prod are valid and
 * combine into dev→stage→prod. Cycles, multiple disconnected roots, or paths
 * that can't be joined into one graph are flagged as errors.
 */
export function validatePromotionChain(paths: PromotionPath[]): PromotionChainValidation {
  if (paths.length === 0) {
    return { valid: false, error: "Pipeline has no promotion paths defined" };
  }

  // Build adjacency map: source -> [targets]
  const graph = new Map<string, string[]>();
  const allTargets = new Set<string>();

  for (const path of paths) {
    const targets = path.targetEnvironmentRefs.map((t) => t.name).filter(Boolean);
    if (targets.length === 0) continue;
    graph.set(path.sourceEnvironmentRef, targets);
    targets.forEach((t) => allTargets.add(t));
  }

  // Single-env pipeline: all paths have no real targets (OC dummy target stripped).
  if (graph.size === 0) {
    return { valid: true, chain: paths.map((p) => p.sourceEnvironmentRef).filter(Boolean) };
  }

  const sources = new Set(graph.keys());

  // Roots are sources that never appear as a target — the entry points.
  const roots = [...sources].filter((s) => !allTargets.has(s));

  if (roots.length === 0) {
    return { valid: false, error: "Promotion paths contain a cycle — no starting environment found" };
  }
  if (roots.length > 1) {
    return {
      valid: false,
      error: `Promotion paths are disconnected — ${roots.length} separate starting environments (${roots.join(", ")})`,
    };
  }

  // Detect cycles via DFS.
  const visited = new Set<string>();
  const onStack = new Set<string>();

  function hasCycle(node: string): boolean {
    visited.add(node);
    onStack.add(node);
    for (const neighbor of graph.get(node) ?? []) {
      if (!visited.has(neighbor)) {
        if (hasCycle(neighbor)) return true;
      } else if (onStack.has(neighbor)) {
        return true;
      }
    }
    onStack.delete(node);
    return false;
  }

  if (hasCycle(roots[0])) {
    return { valid: false, error: "Promotion paths contain a cycle" };
  }

  // hasCycle's DFS visits every node reachable from roots[0]; any source not
  // visited is disconnected from the root — no need for a second traversal.
  // NOTE: visited also includes target-only nodes, so comparing sizes is wrong.
  const unreachable = [...sources].filter((s) => !visited.has(s));
  if (unreachable.length > 0) {
    return {
      valid: false,
      error: `Promotion paths are disconnected — environments not reachable from root: ${unreachable.join(", ")}`,
    };
  }

  // Walk from root following the single outgoing edge — the checks above guarantee a linear chain.
  const chain: string[] = [];
  let current: string | undefined = roots[0];
  while (current) {
    chain.push(current);
    current = (graph.get(current) ?? [])[0];
  }
  return { valid: true, chain };
}
