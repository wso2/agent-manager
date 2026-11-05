# @agent-management-platform/feature-management

Turns console features off by config, and gates them by what the user may do.

`FeatureManagementProvider` props:

| Prop | Example | Unset keys |
| --- | --- | --- |
| `featureEnable` | `{ externally_managed_env: true }` | `false` (feature on) |
| `featureAccess` | `{ env_read: true, env_write: false }` | `true` (allowed) |
| `isAccessLoading` | `true` while the token decodes | `false` |

With no provider, or no props, every feature is on and every access is allowed.
Nested providers inherit from the outer one and override only the keys they set.

## Switches (`featureEnable`)

Only resources that can be managed outside the console have one. `true` hides the
resource's nav item and pages.

| Key | Runtime config env var |
| --- | --- |
| `externally_managed_env` | `FEATURE_EXTERNALLY_MANAGED_ENV` |
| `externally_managed_gateway` | `FEATURE_EXTERNALLY_MANAGED_GATEWAY` |
| `externally_managed_pipeline` | `FEATURE_EXTERNALLY_MANAGED_PIPELINE` |

## Access (`featureAccess`)

Each resource has `<resource>_read` and `<resource>_write` (create, update, delete):
`project`, `agent`, `agent_kind`, `agent_identity`, `env`, `gateway`, `pipeline`,
`git_secret`, `llm_provider`, `llm_provider_template`, `llm_proxy`, `mcp_server`,
`evaluator`, `monitor`, `member`, `role`, `group`.

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

const { canView, canWrite } = useEnvironmentFeatures(); // switch + access
const { canWrite: canEditAgents } = useResourceAccess("agent");
```

`useEnvironmentFeatures`, `useGatewayFeatures` and `usePipelineFeatures` combine the
switch with read/write access; use them for those three resources.

## Where the values come from

The package doesn't know about tokens or scopes. In this console:

- `featureEnable` comes from `window.__RUNTIME_CONFIG__.featureEnable` (Helm:
  `console.config.featureEnable`), overridden by `<CoreUI featureEnable>`.
- `featureAccess` comes from the `useFeatureAccess` hook passed to `<CoreUI>`. The
  web-ui app passes `useScopeAccess` (`apps/web-ui/src/access/`), which maps token
  scopes to access keys. Another app can pass its own hook.

## Add a key

1. Add it to `FeatureEnableConfig` or `FeatureAccessConfig` in `src/types.ts`.
2. For access, map it in the app (`apps/web-ui/src/access/scopeAccess.ts`).
3. Gate the UI with `useResourceAccess` or a hook in `src/features/`.

An app that embeds core-ui can add its own keys by augmenting the interfaces:

```ts
declare module "@agent-management-platform/feature-management" {
  interface FeatureAccessConfig { billing_read: boolean; billing_write: boolean }
}
```
