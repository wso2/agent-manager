# Subscription plans for MCP servers

How an MCP server limits its agents with subscription plans, and what has to be in place for a plan to be enforced. The `ai-hub` module holds the plans and the subscriptions; the `ai-gateway` module offers plans on MCP proxies and sends both to the gateways; the gateway enforces them.

## The pieces

| Piece | Where | What it is |
|---|---|---|
| Plan | `ai-hub`, **Manage › Subscription Plans** or `/m/ai-hub/subscription-plans` | A set of conditions: `tool-call-count`, `request-count` and `token-count`, each with limit windows and `onExceed` (`reject` answers 429, `allow` only logs), and `custom` conditions, each with its own id and key-value settings for a custom policy |
| Offer | `ai-gateway`, the MCP server's **Subscription Plans** tab, or `subscriptionPlans` on the MCP proxy API | The plans an MCP server offers, for every endpoint |
| Subscription | `ai-hub`, copied in from the developer portal | A subscriber's subscription to an MCP server on one of its plans, with a subscription key |
| Policies | The MCP server's endpoint policies | `subscription-validation` v2 checks the key; `subscription-tool-ratelimit` v1 holds the subscriber to the plan's tool-call limits |

## End to end

```
 developer portal                      AI Control Plane                                   AI gateway
 ───────────────                       ────────────────                                   ──────────
 plan (refId = ACP plan handle) ──webhook subscription_plan.*──▶ ai-hub plan ──subscriptionPlan.*──▶ plan
 MCP server (referenceId = ACP MCP proxy handle, offering the plan)
                                       MCP proxy offers the plan, policies attached ──deploy──▶ MCP route
 subscribe ─▶ subscription key
           ──webhook subscription.*──▶ ai-hub subscription ──subscription.*──────────▶ key (hashed)
 agent ── tools/call with Subscription-Key ─────────────────────────────────────────▶ 200, then 429 over the limit
```

1. **The plan.** Make it in the ACP, or in the developer portal, which copies it in by webhook (the portal plan's `refId` is the ACP plan's handle). Every plan reaches the organization's gateways.
2. **The offer.** On the MCP server, tick the plans it offers. A plan offered by an MCP server is in use: it keeps its name and conditions and can't be deleted.
3. **The policies.** Attach `subscription-validation` v2 and `subscription-tool-ratelimit` v1 to every endpoint (the tab offers to when a plan is ticked), and save to deploy them.
4. **The portal's MCP server.** Its `referenceId` is the ACP MCP proxy's handle, and it offers the same plans (by the portal plan's id). The portal lets only one MCP server use a `referenceId`.
5. **Subscribe** in the portal. The portal sends `subscription.created` to the ACP with the key sealed; the ACP stores it (the key in the core's secret store) and sends it to the gateways. A subscribed plan is in use too.
6. **Call** the MCP server with the key in the `Subscription-Key` header.

The developer portal's webhook subscriber for the ACP must take `subscription_plan.*` and `subscription.*` events, at `https://<internal server>/m/ai-hub/webhooks/events`, with the shared secret the `ai-hub` module reads as `MODULE_AI_HUB_WEBHOOK_SECRET`.

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

MCP servers in Agent Manager are organization-level (**MCP Servers** in the console, `/m/ai-gateway/orgs/{org}/mcp-proxies`), and each can offer plans. An agent picks the MCP servers it uses as tools in its configuration (Agent › Configure › MCP tools); the plans the server offers apply to it there.

## The policy catalog

The console lists an MCP server's policies from the policy hub, keeping those every gateway of the organization reports. The policy hub doesn't list `subscription-validation` or `subscription-tool-ratelimit` under MCP yet, so the console adds them, with their gateway version, while every gateway reports them. Once the policy hub lists them, its entries are used.

## What the gateways get

| Change | Gateway event | Pulled on connect from |
|---|---|---|
| Plan created, changed, deleted | `subscriptionPlan.created`, `.updated`, `.deleted` | `GET /api/internal/v1/subscription-plans` |
| Subscription created, changed (plan, status, key), deleted | `subscription.created`, `.updated`, `.deleted`, once for each deployment of the MCP proxy | `GET /api/internal/v1/apis/{deployment id}/subscriptions` |

A gateway holds an MCP proxy as one deployed artifact per endpoint and environment, so a subscription is sent for each. Each copy has its own ID, derived from the subscription's, so a gateway holding two deployments of one proxy keeps both.
