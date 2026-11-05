# @agent-management-platform/feature-management

Console feature flags, and gating by what the user may do.

`FeatureManagementProvider` props:

| Prop | Example | Unset keys |
| --- | --- | --- |
| `featureFlags` | `{ externallyManagedEnv: true }` | `false` |
| `featureAccess` | `{ env_read: true, env_write: false }` | `true` (allowed) |
| `isAccessLoading` | `true` while the token decodes | `false` |

Objects are compared by value, so passing them inline is fine. While access is loading,
pass no `featureAccess` (everything allowed) and set `isAccessLoading`; the nav waits on
it, and pages don't redirect away before the real access is known.

With no provider, every flag is off and every access is allowed. A flag is on only
when its value is exactly `true`.
Nested providers inherit from the outer one and override only the keys they set.

## Flags (`featureFlags`)

All console feature flags live here (`FeatureFlags` in `src/types.ts`); read them with
`useFeatureFlag("enableSelfHostedGateways")`, not from `globalConfig`. In config.js each is
`featureFlags.<name>`, set from a `FEATURE_FLAG_*` env var (Helm:
`console.config.featureFlags`).

Three flags hide a resource that is managed outside the console. They are off by
default, so the pages show:

| Flag | Env var |
| --- | --- |
| `externallyManagedEnv` | `FEATURE_FLAG_EXTERNALLY_MANAGED_ENV` |
| `externallyManagedGateway` | `FEATURE_FLAG_EXTERNALLY_MANAGED_GATEWAY` |
| `externallyManagedPipeline` | `FEATURE_FLAG_EXTERNALLY_MANAGED_PIPELINE` |

## Access (`featureAccess`)

Each resource has `<resource>_read` and `<resource>_write` (create, update, delete):
`project`, `agent`, `agent_kind`, `agent_identity`, `env`, `gateway`, `pipeline`,
`git_secret`, `llm_provider`, `llm_provider_template`, `llm_proxy`, `mcp_server`,
`evaluator`, `monitor`, `role`, `group`.

Some actions have their own permission and so their own key: `gateway_token_write`
(self-hosted gateway registration tokens), and `member_read`, `member_invite` and
`member_remove` (organization users).

Deployments also use `agent_deploy` (non-production environments),
`agent_deploy_production` (production, on top of `agent_deploy`) and `agent_suspend`.

Read gates nav items; write hides create, edit and delete controls. This is a UX
hint only — the server still enforces permissions.

## Use

```tsx
import {
  useEnvironmentFeatures,
  useResourceAccess,
} from "@agent-management-platform/feature-management";

const { canView, canWrite } = useEnvironmentFeatures(); // flag + access
const { canWrite: canEditAgents } = useResourceAccess("agent");
```

`useEnvironmentFeatures`, `useGatewayFeatures` and `usePipelineFeatures` combine the
flag with read/write access; use them for those three resources.

## Where the values come from

The package doesn't know about tokens or scopes. In this console:

- `featureFlags` comes from `window.__RUNTIME_CONFIG__.featureFlags`, overridden by
  `<CoreUI featureFlags>`.
- `featureAccess` comes from the `useFeatureAccess` hook passed to `<CoreUI>`. The
  web-ui app passes `useScopeAccess` (`apps/web-ui/src/access/`), which maps token
  scopes to access keys. Another app can pass its own hook.

## Add a key

1. Add it to `FeatureFlags` or `FeatureAccessConfig` in `src/types.ts` (and, for a
   flag, to `config.template.js` and the Helm chart).
2. For access, map it in the app (`apps/web-ui/src/access/scopeAccess.ts`).
3. Gate the UI with `useResourceAccess` or a hook in `src/features/`.

An app that embeds core-ui can add its own keys by augmenting the interfaces:

```ts
declare module "@agent-management-platform/feature-management" {
  interface FeatureAccessConfig { billing_read: boolean; billing_write: boolean }
}
```
