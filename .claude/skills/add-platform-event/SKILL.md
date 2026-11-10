---
name: add-platform-event
description: Add or change a platform event (the CloudEvents Agent Manager publishes to subscriptions/destinations — webhook, Slack, PagerDuty, …) when a new feature is implemented. Use when a new or changed API operation should be subscribable under Events (e.g. "widget.created"), when a background job should emit an event, or when TestEventCatalogRoutesAreRegistered fails or WithEvents panics with "which is not in the catalog". Covers the event catalog (events/catalog.go), route mapping, scope choice, OmitData for credentials, environment annotation, and non-route publishing.
---

# Add a platform event (agent-manager-service)

**Read first:** `agent-manager-service/docs/events.md` (how events, scopes, subscriptions and delivery work).

Events are CloudEvents 1.0, `type = com.wso2.agentmanager.<name>`. The catalog in `agent-manager-service/events/catalog.go` is the whole registry: subscriptions, the console's event picker (it reads the catalog from the API) and the dispatcher all derive from it. There is no console change needed to make a new type subscribable.

## Step 0 — does it need an event?

Add one when an org admin/project owner would plausibly want to be **notified** that it happened (created/updated/deleted/deployed/state changed/failed). Events report facts, not judgements — no thresholds here.

Skip it for reads, previews, validations, and internal bookkeeping.

## Step 1 — name and scope

- Name: `<resource>.<past_tense_verb>` in snake_case — `widget.created`, `agent.api_key_rotated`. Reuse the existing verb set (`created`, `updated`, `deleted`, `deployed`, `undeployed`, `started`, `stopped`, …). **Names are permanent** once shipped: subscribers store them.
- Scope = the level the event *belongs to*, which decides who can subscribe (see "Which subscriptions an event reaches" in `docs/events.md`):
  - `ScopeOrg` — org-level resources, and creating/deleting a project (`project.deleted` is an org event).
  - `ScopeProject` — things inside a project, including creating/deleting an agent.
  - `ScopeAgent` — things about one agent (deploys, configs, keys, monitors).
  Rule of thumb: creating/deleting X is an event of X's **parent** scope.

## Step 2 — add the type to the catalog

In `events/catalog.go`, add a `Type` to `types` in the right scope section, next to its category:

```go
{"widget.created", ScopeAgent, "Widgets", "A widget was created for the agent."},
```

- `Category` groups types in the console picker — reuse an existing one if it fits.
- `Description` is user-facing: one short sentence, past tense. If the event can still fail afterwards (async rollout) or isn't tied to an environment, say so (see `agent.deployed`, `agent.build_triggered`).

## Step 3a — route events (the common case)

Map the route's **exact registrar pattern** to the type in `routeEvents`:

```go
"POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/widgets": {Type: "widget.created"},
```

`middleware.WithEvents` then publishes it automatically after a 2xx response, with the actor, org, project, agent and the response body as `data`.

- **`OmitData: true`** if the response contains any credential (API key, token, client secret, webhook secret, invite link). The body must never leave the platform in an event. When in doubt, omit.
- **Environment:** agent events tied to an environment must carry it. It is read from the path (`{envID}`), the response fields `environment` / `environmentName` / `targetEnvironment`, or set explicitly in the controller/service:
  ```go
  events.SetEnvironment(ctx, env.Name)
  ```
  Set `EnvironmentQuery: true` if the route takes `?environment=`.
- Extra data when the body is omitted or lacks it: `events.AddData(ctx, "name", resp.KeyID)` (never secrets).
- Several routes may share a type (`agent.configuration_updated`).

Then:

```bash
cd agent-manager-service && go test ./events/... ./api/ -run 'Event|Catalog'
```

`TestEventCatalogRoutesAreRegistered` fails if a pattern isn't a registered route or emits a type missing from the catalog; `WithEvents` panics at startup for the latter.

## Step 3b — non-route events (background jobs, schedulers, async outcomes)

Add a `Type…` constant next to `TypeMonitorRunSucceeded` in `catalog.go`, list it in `types`, and publish through the injected `events.Publisher` (never the bus directly — the publisher writes the durable outbox first):

```go
e := events.New("evt_"+uuid.NewString(), events.TypeWidgetSynced, events.ScopeAgent,
    orgID, projectName, agentName, at)
e.Environment = envName
e.Data = payload            // no secrets
if err := s.publisher.Publish(ctx, e); err != nil { /* log; decide retry */ }
```

Follow `services/monitor_run_events.go` for a worked example. If the outcome must not be lost, use a **deterministic event ID** and replay until `Publish` succeeds (receivers dedupe by `webhook-id` = event ID). Wire the publisher via `wiring/wire.go` providers; tests can use `events.NoopPublisher{}` or a recording fake.

## Step 4 — tests

- Catalog: add cases to `events/catalog_test.go` if scope/reach behaviour is non-obvious.
- Service-level publishing: unit test that the expected event (type, scope, env, data without secrets) is published — see `services/*_unit_test.go` and `/add-service-unit-test`.

## Step 5 — docs and console

- Update `docs/events.md` only if behaviour (not just a new type) changes.
- No console change is needed for the picker. If the console renders type-specific labels/icons, check `console/workspaces/pages/settings/src/events/` and `libs/types/src/api/events.ts`.

## Checklist

- [ ] Name `<resource>.<past_verb>`, correct scope (create/delete → parent scope)
- [ ] `types` entry with category + user-facing description
- [ ] `routeEvents` entry with exact pattern, or `Publisher.Publish` for non-route events
- [ ] `OmitData` for any response carrying credentials
- [ ] Environment carried for environment-bound agent events
- [ ] `go test ./events/... ./api/...` passes
- [ ] Also consider `/add-feature-telemetry` (usage analytics) and `/add-audit-event` (security-relevant operations) for the same feature
