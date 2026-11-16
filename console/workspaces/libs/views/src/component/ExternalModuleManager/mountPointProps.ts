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

// Prop contracts for overridable mount-point components. These describe what the
// built-in consumer passes to an injected component, so a deployment can implement a
// drop-in replacement against a stable interface.

/** Repository source fields a private-repo-source component reads and writes. */
export interface PrivateRepoSourceValues {
  repositoryUrl: string;
  branch: string;
  appPath: string;
  /** Name of the selected git secret; empty/undefined for public or app-managed repos. */
  gitSecretRef?: string;
}

/** Repository source metadata submitted atomically with agent creation. */
export interface GitHubAppSourceBinding {
  installationId: number;
  owner: string;
  repo: string;
  branch: string;
  appPath: string;
  repositoryUrl: string;
}

/**
 * Props passed to a component injected at MountPoints.PrivateRepoSource. A deployment
 * uses this to replace the built-in personal-access-token git-secret UI with an
 * alternative repository connection flow (for example a GitHub App). The component
 * renders the repository source inputs, reads current values, and writes changes back
 * through onFieldChange. When no component is injected, the built-in PAT UI renders.
 */
export interface PrivateRepoSourceProps {
  /** Current repository source values from the parent form. */
  values: PrivateRepoSourceValues;
  /** Per-field validation errors from the parent form. */
  errors: Partial<Record<keyof PrivateRepoSourceValues, string | undefined>>;
  /** Writes a single repository source field back to the parent form. */
  onFieldChange: (field: keyof PrivateRepoSourceValues, value: string) => void;
  /** Current injected source binding; omitted by the built-in PAT flow. */
  sourceBinding?: GitHubAppSourceBinding;
  /**
   * Validation error for the source binding as a whole (any of its fields), so the
   * component can show why submission is blocked. Undefined when the binding is valid.
   */
  sourceBindingError?: string;
  /** Updates the binding carried by the create request; this performs no network I/O. */
  onSourceBindingChange: (binding: GitHubAppSourceBinding | undefined) => void;
  /** Organization id (handle) for scoping repository and secret lookups. */
  orgId: string;
  /** Project (id/name) the component belongs to. */
  projectName?: string;
  /**
   * Component (agent) name the source is for. On the create form this is the in-progress
   * name. May be empty until the user names the component.
   */
  componentName?: string;
  /** When true, inputs should be disabled (for example while a submit is in flight). */
  disabled?: boolean;
}
