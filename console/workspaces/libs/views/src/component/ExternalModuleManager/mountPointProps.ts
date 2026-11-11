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

/** Replica restriction reported by a component injected at MountPoints.ReplicaPolicy. */
export interface ReplicaPolicy {
  /** Highest replica count allowed, for both fixed replicas and the autoscaling max. */
  maxReplicas: number;
  /** When false, autoscaling cannot be turned on. */
  autoScalingAllowed: boolean;
}

/**
 * Props passed to a component injected at MountPoints.ReplicaPolicy. The component
 * renders inside the resource configurations drawer (for example a notice explaining
 * the restriction) and reports the policy through onPolicyChange. Until it reports,
 * the drawer does not allow saving.
 */
export interface ReplicaPolicyProps {
  /** Organization id (handle) the agent belongs to. */
  orgId: string;
  /** Reports the restriction to apply, or null when replicas are unrestricted. */
  onPolicyChange: (policy: ReplicaPolicy | null) => void;
}

/** An organization the user can move the console to. */
export interface OrganizationSwitcherOption {
  /** Identifies the organization to onSwitch. */
  id: string;
  /** Name shown in the header's organization switcher. */
  name: string;
}

/** Organization switching reported by a component injected at MountPoints.OrganizationSwitcher. */
export interface OrganizationSwitcherState {
  /** The organizations the user can be in, the current one included. */
  organizations: OrganizationSwitcherOption[];
  /** The id of the organization the console is in. */
  currentId: string;
  /** The id of the organization the console is moving to, while a switch runs. */
  switchingId?: string;
  /**
   * Moves the console to the organization with this id. The component owns the whole
   * switch, including loading the console in the new organization once it has moved.
   */
  onSwitch: (id: string) => void;
}

/**
 * Props passed to a component injected at MountPoints.OrganizationSwitcher. A deployment
 * whose sign-in is bound to one organization at a time (so the API lists only that one)
 * uses this to offer the user's other organizations in the header's organization
 * switcher. The component renders in the header (nothing, or its own notice such as a
 * failed switch) and reports through onSwitcherChange. Until it reports, and whenever it
 * reports null, the header lists the organizations the API returns.
 */
export interface OrganizationSwitcherProps {
  /** Reports the organizations to offer, or null to leave the header's own list. */
  onSwitcherChange: (switcher: OrganizationSwitcherState | null) => void;
}
