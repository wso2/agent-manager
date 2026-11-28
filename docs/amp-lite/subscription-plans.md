# Subscription plans for MCP servers

How an MCP server limits its agents with subscription plans, and what has to be in place for a plan to be enforced. The `ai-hub` module holds the plans and the subscriptions; the `ai-gateway` module offers plans on MCP proxies and sends both to the gateways; the gateway enforces them.

## The pieces

| Piece | Where | What it is |
|---|---|---|
| Plan | `ai-hub`, **Manage › Subscription Plans** or `/m/ai-hub/subscription-plans` | A set of conditions: `tool-call-count`, `request-count` and `token-count`, each with limit windows and `onExceed` (`reject` answers 429, `allow` only logs), and `custom` conditions, each with its own id and key-value settings for a custom policy |
| Offer | `ai-gateway`, the MCP server's **Subscription Plans** tab, or `subscriptionPlans` on the MCP proxy API | The plans an MCP server offers, for every endpoint |
| Subscription | `ai-hub`, copied in from the developer portal | A subscriber's subscription to an MCP server on one of its plans, with a subscription key |
| Policies | The MCP server's endpoint policies | `subscription-validation` v2 checks the key; `subscription-tool-ratelimit` v1 holds the subscriber to the plan's tool-call limits |

## A plan as a document

The plans API (`/m/ai-hub/subscription-plans`) serves and takes each plan as a document, the shape Kubernetes resources have, so a plan can be kept in a file and applied as well as edited in the console:

```yaml
apiVersion: ai-control-plane.choreolabs.com/v1   # the document schema's version, separate from the URL's
kind: SubscriptionPlan                           # must match the route
metadata:
  name: gold                 # the plan's handle; it can't change
  uid: 0b9e...               # set by the server, like the timestamps and createdBy
  origin: ai-control-plane   # where the plan was made: here, or the developer portal that copied it in
spec:
  displayName: Gold
  description: For production agents
  status: ACTIVE             # ACTIVE or INACTIVE
  expiryTime: 2027-01-01T00:00:00Z
  conditions:
    - type: tool-call-count
      params:
        onExceed: reject
        limits: [{count: 30, timeAmount: 1, timeUnit: MINUTE}]
status:                      # observed by the server, ignored on input
  deprecated: false          # with deprecationTimestamp once deprecated
  usage: {inUse: true, mcpProxies: 1, subscriptions: 3}   # GET of one plan only
```

- Bodies are JSON, or YAML with `Content-Type: application/yaml`. A response is YAML when the request's `Accept` names `application/yaml` first.
- `PUT /subscription-plans/{name}` takes the same document; `metadata.name` must be the plan's handle. Spec fields left out are kept, so a document read with GET can be sent back as it is, and `spec.expiryTime: null` clears the expiry.
- A list is `{list, count, pagination}`, each item a whole document; its `status` has no `usage`. Current plans come first, then deprecated ones, each newest first, on every page.
- The older single-limit form is still taken as `spec.limits`, and read as a request-count condition; it isn't returned.

## Changing and deprecating a plan

- **In use.** A plan with subscriptions is in use: it keeps its name, conditions and expiry (`409 SUBSCRIPTION_PLAN_IN_USE`); its description and status (`ACTIVE`/`INACTIVE`) can still change, and are only saved here. Offering a plan on an MCP server doesn't put it in use. To change what subscribers get, make a new plan.
- **No delete, only deprecate.** `DELETE /subscription-plans/{name}` (the console's **Deprecate**) is a soft delete, allowed in use or not, and answers with the deprecated plan. A deprecated plan stays, read only, for the subscriptions on it:
  - it can't be changed, its status included (`409 SUBSCRIPTION_PLAN_DEPRECATED`), and its handle can't be used again;
  - it can't be offered on an MCP server. An MCP server that already offers it keeps it, but its plans can't change otherwise until it is removed (unticked); saving the server's other settings still works;
  - it takes no new subscriptions, and no subscription can move onto it; the developer portal's delivery is refused with `409 SUBSCRIPTION_PLAN_DEPRECATED`, which it doesn't retry. Subscriptions already on it keep working and can still change;
  - gateways still get it, with its subscriptions, and pull it when they connect.
- **Origin.** A plan made here has the origin `ai-control-plane`; one the developer portal copies in has the portal's name, `developer-portal` unless the module's `webhook_source_name` setting (`MODULE_AI_HUB_WEBHOOK_SOURCE_NAME`) names it. The portal can't create a plan with the handle of a plan made elsewhere (`409 SUBSCRIPTION_PLAN_ORIGIN_CONFLICT`).
- **From the developer portal.** The portal creates, changes and deletes its plans by webhook (`subscription_plan.*`). A deletion there deprecates the plan here; a later change of it is ignored, and a plan made there again with its handle is refused (`409`).

## End to end

```
 developer portal                      AI Control Plane                                   AI gateway
 ───────────────                       ────────────────                                   ──────────
 plan (refId = ACP plan handle) ──webhook subscription_plan.*──▶ ai-hub plan
 MCP server (referenceId = ACP MCP proxy handle, offering the plan)
                                       MCP proxy offers the plan, policies attached ──deploy──▶ MCP route
 subscribe ─▶ subscription key
           ──webhook subscription.*──▶ ai-hub subscription ──subscriptionPlan.created─────────▶ plan
                                                             then subscription.*──────────────▶ key (hashed)
 agent ── tools/call with Subscription-Key ─────────────────────────────────────────▶ 200, then 429 over the limit
```

1. **The plan.** Make it in the ACP, or in the developer portal, which copies it in by webhook (the portal plan's `refId` is the ACP plan's handle). A plan doesn't reach the gateways when it is made or changed: it reaches them with its first subscription.
2. **The offer.** On the MCP server, tick the plans it offers. Only an active plan that isn't deprecated can be added; the tab marks an offered plan that has since been deactivated or deprecated.
3. **The policies.** Attach `subscription-validation` v2 and `subscription-tool-ratelimit` v1 to every endpoint (the tab offers to when a plan is ticked), and save to deploy them.
4. **The portal's MCP server.** Its `referenceId` is the ACP MCP proxy's handle, and it offers the same plans (by the portal plan's id). The portal lets only one MCP server use a `referenceId`.
5. **Subscribe** in the portal. The portal sends `subscription.created` to the ACP with the key sealed; the ACP stores it (the key in the core's secret store) and sends its plan, then the subscription, to the gateways. A subscribed plan is in use. A subscription to a deprecated plan is refused.
6. **Call** the MCP server with the key in the `Subscription-Key` header.

The developer portal's webhook subscriber for the ACP must take `subscription_plan.*` and `subscription.*` events, at `https://<internal server>/m/ai-hub/webhooks/events`, signed with one of the organization's **webhook secrets**. Each delivery names its secret by `secret_hash`, and the organization by `org.ref_id`; anything else is refused (`401`). There is no secret shared by every organization: until an organization makes a webhook secret, no delivery reaches it.

- **Make one** for each portal that sends to the organization, or each portal organization: `POST /m/ai-hub/webhook-secrets` with `id`, `name` and, when the portal sends something other than the ACP organization's id, `orgRef` (permission `ai-hub:webhook-secret-create`). The value is returned once; set it on the portal's webhook subscriber.
- **The delivery is for** the organization that made the secret. `org.ref_id` must be its id or the secret's `orgRef`, so a portal organization whose control plane reference is set for another control plane (such as that control plane's organization handle) can still send here. A secret never signs for another organization.

So one organization can take deliveries from several portals (a secret each), each organization of one portal can send to its own organization here, and two organizations here can each map the same portal reference with their own secret.

**Rotate** a webhook secret with `POST /m/ai-hub/webhook-secrets/{id}/rotate`: it adds a version and returns its value once. The older versions keep verifying deliveries that name them by `secret_hash`, so the portal can switch to the new value with no refused delivery; then delete the old version (`DELETE …/{id}/versions/{version}`). A secret keeps at most `webhook_secret_max_versions` versions (3 by default, at least 2; `409 WEBHOOK_SECRET_VERSION_LIMIT` until one is deleted), and its last version can't be deleted on its own (delete the secret). The values are kept in the core's secret store; the module's tables hold only their hashes and masked forms. Deliveries with no `secret_hash`, from a portal that predates it, are refused unless `webhook_legacy_shared_secret` is `true`; they are then checked against the webhook secrets of the organization they name (at most 32, newest first; when they name an organization by its id, its own secrets come before those other organizations map to it). A legacy sender's old version must stay among those 32 while it rotates: a version pushed out by newer ones no longer verifies its deliveries, though it is still kept. Set `webhook_enabled` to `false` to refuse every delivery. A webhook setting that is set but invalid (`webhook_enabled: flase`, say) stops the module from activating, rather than leaving a default in place. Every webhook setting, with its default, is in the [configuration reference](configuration.md#module-ai-hub).

## What must be in place, and what happens if it isn't

| If… | Then… |
|---|---|
| The MCP server offers plans but has **no subscription policies** | Nothing is enforced: every call goes through as before. Offering a plan doesn't attach the policies; the tab warns while they are missing. |
| `subscription-validation` v2 is attached | **Calls without an active subscription get 403**, existing agents included, until they subscribe. This is intended: whoever attaches the plans decides when subscriptions start to count, so there is no default plan. |
| Only `subscription-validation` **v1** is attached | It checks the key but records no plan, so the rate-limit policy after it limits nothing. Use v2. |
| `subscription-tool-ratelimit` is missing | Subscribers are checked but not limited. |
| A gateway doesn't have the policies | The console doesn't offer them: the policy list (MCP server › Policies) shows only policies every gateway of the organization reports in its manifest. The gateway images from `2026.09.24` on ship both. |
| A plan has **request** limits | They aren't enforced on MCP servers: every MCP protocol message (`initialize`, `tools/list`, pings) is a request. Only tool-call limits are. |
| A plan has **token** conditions | Not enforced on MCP servers: MCP calls carry no LLM tokens. |
| A plan has **custom** conditions | Enforced by a custom policy you provide and attach to the endpoint after `subscription-validation` v2, which puts the subscriber's plan conditions, custom ones included, in the request's metadata (`x-wso2-subscription-plan-conditions`) for the policy to read. Without such a policy, a custom condition limits nothing. |
| The `ai-hub` module is disabled | The gateways keep what they have until they reconnect, then pull no plans and no subscriptions, and calls get 403 while `subscription-validation` is attached. |

## Which MCP servers

MCP servers in Agent Manager are organization-level (**MCP Servers** in the console, `/m/ai-gateway/mcp-proxies`, where `spec.subscriptionPlans` lists the plans a proxy offers), and each can offer plans. An agent picks the MCP servers it uses as tools in its configuration (Agent › Configure › MCP tools); the plans the server offers apply to it there.

## The policy catalog

The console lists an MCP server's policies from the policy hub, keeping those every gateway of the organization reports. The policy hub doesn't list `subscription-validation` or `subscription-tool-ratelimit` under MCP yet, so the console adds them, with their gateway version, while every gateway reports them. Once the policy hub lists them, its entries are used.

## What the gateways get

| Change | Gateway event | Pulled on connect from |
|---|---|---|
| Subscription created or changed (plan, status, key) | Its plan first, as `subscriptionPlan.created` (a gateway stores it, or replaces the one it has); then `subscription.created` or `.updated`, once for each deployment of the MCP proxy | Plans: `GET /api/internal/v1/subscription-plans`, only those a subscription to a deployed MCP proxy is on. Subscriptions: `GET /api/internal/v1/apis/{deployment id}/subscriptions` |
| Subscription deleted | `subscription.deleted`, once for each deployment; the plan stays until the gateway next connects | |
| Plan created, changed or deprecated, here or in the developer portal | Nothing: plans are subscribed to in the developer portal, so a plan reaches the gateways with its subscriptions. A deprecated plan still does, for the subscriptions on it | |

A gateway holds an MCP proxy as one deployed artifact per endpoint and environment, so a subscription is sent for each. Each copy has its own ID, derived from the subscription's, so a gateway holding two deployments of one proxy keeps both.
