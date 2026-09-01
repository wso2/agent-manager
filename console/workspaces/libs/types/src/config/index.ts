/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

import type { AsgardeoProviderProps } from "@asgardeo/react";
import { TraceListTimeRange } from "../api/traces";
import { type Duration, sub } from "date-fns";
export interface AppConfig {
  authConfig: AsgardeoProviderProps;
  apiBaseUrl: string;
  /**
   * Base URL for the unauthenticated GET /api/v1/config discovery request that
   * runs at app bootstrap (before the user has a token)
   */
  configDiscoveryBaseUrl?: string;
  /** Gateway control plane URL (default: http://localhost:9243). Used for gateway setup commands. */
  gatewayControlPlaneUrl?: string;
  /** Gateway version used in setup commands (default: v0.9.0). */
  gatewayVersion?: string;
  /**
   * Agent Manager release version (e.g. v0.15.0); pins the deployment-script
   * git tag (amp/vX.Y.Z).
   */
  ampVersion?: string;
  disableAuth: boolean;
  instrumentationUrl: string;
  /**
   * When true, the OTEL endpoint shown in the Setup Agent panel is
   * `instrumentationUrl` as-is. Enable this in deployments where one endpoint
   * fronts every environment, or where the gateway vhost is not externally
   * reachable. Defaults to false: the endpoint is derived per environment from
   * the vhost of the gateway mapped to that environment.
   */
  useConfiguredInstrumentationUrl?: boolean;
  /**
   * Base URL the API Platform Gateway uses to reach Agent Manager from inside
   * the cluster. The add-environment.sh script appends /api/v1 and
   * /auth/external/jwks.json to this as needed.
   * Used to render the curl|bash command in the Create Environment drawer.
   */
  agentManagerInternalBaseUrl?: string;
  /**
   * host:port the API Platform Gateway uses to reach Agent Manager's
   * internal control-plane channel (port 9243). Used to render the
   * add-environment.sh command in the Create Environment drawer.
   */
  agentManagerInternalCpHost?: string;
  /**
   * Base domain env-Thunder instances are hosted under (e.g. amp.localhost,
   * or a VM/production deployment's own domain). Mirrors the backend's
   * IDP_HOST_BASE_DOMAIN. Used to render the correct
   * IDP_HOST_BASE_DOMAIN value in the Create Environment drawer's
   * generated add-environment.sh command, and to build the handle preview.
   */
  idpHostBaseDomain?: string;
  /**
   * Whether this deployment serves env-Thunder over TLS. Mirrors the
   * backend's TLS_ENABLED. Piped into the Create Environment drawer's
   * generated command so the chained add-environment-thunder.sh script
   * provisions the matching scheme.
   */
  tlsEnabled?: boolean;
  guardrailsCatalogUrl: string;
  guardrailsDefinitionBaseUrl: string;
  /**
   * Capability flags that unlock guardrail policies requiring external system configuration.
   * OOTB policies are always shown regardless of these flags.
   */
  guardrailCapabilities?: GuardrailCapabilities;
  /** URL for the product documentation. Shown as a "Docs" link in the footer. */
  docsUrl?: string;
  /** URLs rendered in the footer. */
  footerLinks?: {
    privacyPolicyUrl?: string;
    termsOfUseUrl?: string;
  };
  /** Documentation deep-link paths for AMP instrumentation, appended to docsUrl. */
  instrumentationDocLinks?: {
    /** Path to the manual instrumentation contract section. */
    manualInstrumentation?: string;
    /** Path to the AMP instrumentation version mapping section. */
    versionMapping?: string;
  };
  /** Feature flags. All default to false (disabled) unless explicitly enabled. */
  featureFlags?: FeatureFlags;
}

export type FeatureFlags = {
  /** Shows the private Git repository option when building agents from source. */
  enablePrivateRepoSupport?: boolean;
  /**
   * When true, identity provider management calls the REST API directly instead of
   * rendering the self-hosted manage-identity-provider.sh script snippet.
   */
  enableIdentityProviderManagedMode?: boolean;
  /**
   * When false, the Profile Settings text fields and Save Changes button are
   * hidden/disabled, and the Change Password tab is disabled.
   */
  enableProfileManagement?: boolean;
  /**
   * When false, the Add User, Invite User, Create Role, and Create Group
   * buttons on the Settings page are disabled.
   */
  enableUserManagement?: boolean;
  /**
   * Shows the Agent ID surfaces: the per-agent Agent ID page, the
   * organization-level identity groups/roles pages, and the roles & groups
   * section on the agent overview.
   */
  enableAgentIdentity?: boolean;
};

export type GuardrailCapabilities = {
  /** Unlocks: aws-bedrock-guardrail */
  awsBedrock?: boolean;
  /** Unlocks: azure-content-safety-content-moderation */
  azureContentSafety?: boolean;
  /** Unlocks: granite-guardian-prompt-injection */
  graniteGuardian?: boolean;
  /** Unlocks: nvidia-nemoguard-content-safety */
  nemoGuard?: boolean;
  /** Unlocks: semantic-prompt-guard, semantic-cache */
  semanticGuardrails?: boolean;
};

// Extend the Window interface to include our config
declare global {
  interface Window {
    __RUNTIME_CONFIG__: AppConfig;
  }
}

export const globalConfig: AppConfig = window.__RUNTIME_CONFIG__;

/**
 * Whether the Agent ID surfaces are shown. Read through this rather than the
 * flag directly — the feature spans the nav, routes and the agent overview, and
 * every "value missing" path (older hand-written config.js, unsubstituted
 * template placeholder) has to land on disabled.
 */
export const isAgentIdentityEnabled = (): boolean =>
  globalConfig.featureFlags?.enableAgentIdentity === true;

const buildRange = (duration: Duration) => {
  const endTime = new Date();

  return {
    startTime: sub(endTime, duration).toISOString(),
    endTime: endTime.toISOString(),
  };
};

export const getTimeRange = (timeRange: TraceListTimeRange) => {
  switch (timeRange) {
    case TraceListTimeRange.TEN_MINUTES:
      return buildRange({ minutes: 10 });
    case TraceListTimeRange.THIRTY_MINUTES:
      return buildRange({ minutes: 30 });
    case TraceListTimeRange.ONE_HOUR:
      return buildRange({ hours: 1 });
    case TraceListTimeRange.THREE_HOURS:
      return buildRange({ hours: 3 });
    case TraceListTimeRange.SIX_HOURS:
      return buildRange({ hours: 6 });
    case TraceListTimeRange.TWELVE_HOURS:
      return buildRange({ hours: 12 });
    case TraceListTimeRange.ONE_DAY:
      return buildRange({ days: 1 });
    case TraceListTimeRange.THREE_DAYS:
      return buildRange({ days: 3 });
    case TraceListTimeRange.SEVEN_DAYS:
      return buildRange({ days: 7 });
    case TraceListTimeRange.THIRTY_DAYS:
      return buildRange({ days: 30 });
  }
};
