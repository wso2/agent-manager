# Platform events: destinations and subscriptions

Agent Manager publishes a platform event when something happens: a project is created, an agent is deployed, a monitor run finishes. **Subscriptions** pick events and send them to **destinations** — a Slack channel, a PagerDuty service, a webhook — that org admins register. The console shows this under **Events**, at the org, on each project and on each agent. This document covers how it works and how to run it.

## How it works

```
API (2xx) ─► events middleware ─► Publisher ─► events.Bus ─► AMP_EVENTS
Monitor scheduler ───────────────────┘                          │
                                                                ▼
                      event dispatcher (inside the service, every replica)
                        fan-out: match subscriptions, group them by destination,
                                 record "pending", one job per destination
                                 ─► AMP_EVENT_DELIVERIES
                        delivery: destination driver sends, retries, records result
                                                                │
                                                                ▼
                                   webhook · notify (Slack, Teams, email, PagerDuty, …)
```

- **Events** are CloudEvents 1.0 (`type` = `com.wso2.agentmanager.<name>`), with extension attributes `orgid`, `scope`, `project`, `agent`, `environment`, `actortype` and `actorid`. The catalog in `events/catalog.go` maps API routes to event types and scopes. Route events fire when a request succeeds.
- **The event bus** sits behind the `events.Bus` interface. NATS JetStream is the implementation shipped, as a server (`nats`) or embedded in the process (`embedded`).
- **Destinations** belong to the org. Only org admins register them. Each has a **kind** that decides how events are delivered, with a driver behind `eventdestinations.Driver`:
  - `webhook` POSTs the CloudEvent signed per [Standard Webhooks](https://www.standardwebhooks.com) (`webhook-id`, `webhook-timestamp`, `webhook-signature`), refusing private addresses.
  - `notify` sends a short text notification (title, description, where, when) to a chat, email, paging, push or SMS service, through [Shoutrrr](https://github.com/nicholas-fedor/shoutrrr) (MIT). Shoutrrr's maintainers keep each service's integration current; the driver (`services/eventdestinations/notify`) only picks which services are offered and keeps their traffic safe. A destination is a service plus a Shoutrrr service URL, such as `slack://hook:<token>@webhook`. The URL carries the service's token, so it is stored encrypted like a webhook secret and never returned; leave it out on update to keep it.
  - **Availability** decides which subscriptions below the org may use a destination: those of every project (`all`), none (`org`), or those of listed projects (`projects`). Org subscriptions may use any destination. Narrowing availability while subscriptions would lose the destination, or deleting a destination subscriptions use, is refused (409) with the subscriptions named.
- **Subscriptions** belong to the org, a project or an agent, and pick event types, a destination and an optional environment filter.
- **The dispatcher** is a module of the service, not a separate deployment. Every replica runs it on shared durable consumers (`event-fanout`, `event-delivery`), so each message is handled by one replica.

## Which subscriptions an event reaches

Scopes rank org, project, agent. A subscription receives an event when all of these hold:

1. **Scope.** The event happens at the subscription's level or a narrower one, inside it. An org subscription gets the events of every project and agent; a project subscription those of the project and its agents; an agent subscription those of the agent. An event never reaches a narrower subscription, even when it names it: `project.deleted` is an org event that names the project, so the project's subscriptions do not get it, and `agent.created` is a project event, so the agent's do not.
2. **Type.** The subscription names the event type, or names only `*`. `*` is every type the subscription's level can receive, including types added later; it never matches the destination test event.
3. **Environment.** The filter is empty (every environment), or the event is tied to no environment (a build, say), or the event's environment is in the filter.
4. **Enabled**, and its destination is still available to it.

The event goes to each destination once, however many subscriptions send it there; the delivery records them all.

Lower levels see the wider levels' subscriptions that reach them, read-only and narrowed to the types that can happen there, so owners do not duplicate them.

## Delivery guarantees

- **Durable acceptance.** Events accepted by the publisher are stored in PostgreSQL's `event_outbox` before returning. A background worker leases rows, publishes to NATS, and removes them only after confirmation. Bus failures retry without a fixed attempt limit; interrupted leases expire after two minutes.
- **Monitor replay.** Terminal runs remain replayable until their complete event (including scores) is accepted by the outbox. A score-query or outbox-write failure retries on a later scheduler tick. Replay retains the run's completion time and event ID.
- **Publication boundary.** Monitor outcomes have a durable source. Other route events are recorded after the operation completes; a crash or database failure between that operation and outbox insertion is still outside the delivery guarantee. The route logs an explicit persistence failure.
- **At least once.** A receiver may see an event twice after a crash or a lost acknowledgement; it should drop duplicates by `webhook-id`, which is the same on every attempt.
- **Retries** follow the driver: a webhook retries network errors, 408, 425, 429 and 5xx after 10s, 1m, 5m, 30m and 2h, up to `EVENTS_DELIVERY_MAX_ATTEMPTS`. Other responses fail at once. Shoutrrr reports no status codes, so a failed `notify` send is always retried; an invalid URL is refused when the destination is saved.
- **A delivery already recorded as delivered is not sent again** when its job comes back. A worker claims a delivery for two minutes before sending it, so a job the bus redelivers while another worker is still sending waits instead of sending twice; and a send that succeeded is not retried even when recording its outcome fails.
- **A delivery nobody wants any more is skipped.** Before each attempt the dispatcher checks that a subscription that queued it still exists, is enabled and may use the destination; otherwise it records `skipped` and stops.
- **With `embedded`**, the bus is in memory: events and pending retries waiting at a restart are lost. Use `nats` for anything but quick start.
- **History** is kept for `EVENTS_DELIVERY_RETENTION_DAYS` in `event_deliveries`. Event bodies are not stored there; the event lives on the bus (72 hours on `AMP_EVENTS`). Each level's delivery log shows its own events: the org every delivery, a project its project and agent events, an agent its agent events.

## Configuration

| Setting | Default | Purpose |
|---|---|---|
| `EVENTS_MODE` | `off` (the chart sets `nats`) | `nats`, `embedded` or `off`. `off` runs no bus and the console hides Events. |
| `EVENTS_NATS_URL` | – | NATS server for `nats` mode |
| `EVENTS_NATS_TOKEN` | – | Token for a NATS server that requires one |
| `EVENTS_DELIVERY_MAX_ATTEMPTS` | `6` | Attempts before a delivery is marked failed |
| `EVENTS_DELIVERY_RETENTION_DAYS` | `14` | Delivery history retention |
| `EVENTS_ALLOW_PRIVATE_DESTINATIONS` | `false` | Allow private or plain-http destination URLs. Local development only. |

`embedded` supports a single replica; several replicas, or autoscaling, need `nats`. The console learns whether Events is on from `GET /api/v1/config` (`eventsEnabled`).

### Helm

`agentManagerService.config.events.mode` is the only switch. With `nats` and no `events.natsUrl`, the chart deploys a NATS StatefulSet (`amp-nats`, JetStream on a 2Gi volume); with `natsUrl` set it uses that server instead. The bundled NATS requires a token the chart generates (Secret `amp-nats-auth`), and a NetworkPolicy (`nats.networkPolicy.enabled`) lets only the service connect: the bus carries every org's events. For an external server that requires a token, name its Secret in `events.natsToken.existingSecret`. The chart refuses `embedded` with more than one replica or with autoscaling on. `embedded` and `off` deploy no NATS. The quick-start installer uses `embedded`. `events.deliveryMaxAttempts`, `events.deliveryRetentionDays` and `events.allowPrivateDestinations` set the delivery settings.

### docker-compose

`make setup` runs a NATS container and sets `EVENTS_MODE=nats`, with private destinations allowed so a receiver on your machine can be used.

## Permissions

| What | View | Change |
|---|---|---|
| Destinations, org subscriptions, org delivery log | `amp:events:read` | `amp:events:manage` |
| Project subscriptions and delivery log | project read | project update |
| Agent subscriptions and delivery log | agent read | agent update |

Project and agent callers see only the destinations available to them, as name, kind and health — never their settings. Callers without `amp:events:manage` see destination settings masked. Only `amp:events:manage` can test a destination, so a project cannot page the org's on-call. Every change is written to the audit trail (`event-destination:*`, `event-subscription:*`), and subscriptions are removed with their project or agent; a deleted project is also taken off the destinations made available to it.

## Notification services

`GET /orgs/{org}/events/destination-kinds` lists the kinds and, for `notify`, the offered services with their URL format and a link to Shoutrrr's documentation. The console builds its form from it.

Offered: Slack, Microsoft Teams, Google Chat, Discord, Telegram, Mattermost, Rocket.Chat, Zulip, Email (SMTP), PagerDuty, Opsgenie, ntfy, Gotify, Pushover and Twilio SMS.

- **Adding a service** Shoutrrr supports is one entry in `offered` (`notify.go`) and a sample URL in its test. Offer only services that send through the client the driver supplies (`types.HTTPClientSetter`); the test fails otherwise.
- **SSRF.** HTTP services send through the same SSRF-hardened client as webhooks. Hosts named in a self-hosted service's URL (Mattermost, Gotify, an SMTP relay, …) must resolve to public addresses when the destination is saved; SMTP does not go over HTTP, so its connection resolves and validates every address, then dials a validated IP directly while retaining the original hostname for TLS and authentication. `EVENTS_ALLOW_PRIVATE_DESTINATIONS` lifts both checks for local development.
- **Upgrading Shoutrrr.** Releases from v0.19.0 need Go 1.27; the service pins v0.18.0 until it moves to Go 1.27. Update `libraryVersion` in `notify.go` with it, which versions the docs links.

## Adding a destination kind

1. Implement `eventdestinations.Driver` in `services/eventdestinations/<kind>/`: validate and normalise the settings, mask them for read-only callers, generate a secret if the kind has one, deliver, and say which failures are retryable.
2. Register it in `ProvideDestinationDrivers` (`wiring/wire.go`).
3. Add the kind's settings fields to the console's destination form (`DestinationForm.tsx`).

A kind whose settings include a credential the user enters implements `eventdestinations.CredentialDriver` too; one that delivers to several services implements `eventdestinations.ServiceCatalog`.

Matching, queuing, retries, the delivery log and the API stay as they are.


## Upgrading event durability

Migration 055 adds the outbox and `monitor_runs.event_queued_at`. It preserves existing rows and marks pre-upgrade terminal runs as already handled; pending/running and newly created runs remain replayable. New workers coexist through expiring leases. Old application replicas still use their original publication behavior, so complete the rollout before relying on durable acceptance.

Rollback leaves the additive table and column in place, preserving pending work for a subsequent upgrade. Start the older binary without running its migration command (its migration validator rejects newer schema versions). Do not drop pending outbox rows. Embedded mode still loses events already published into its in-memory bus on restart; use NATS mode for durable downstream delivery.

## Monitor alerting

Evaluation jobs publish `mean` and `pass_rate` for every evaluator with scores, alongside any evaluator-specific aggregations. Pass rate is the fraction of non-skipped scores greater than or equal to 0.7, using the evaluation SDK's standard pass threshold. An alert rule compares that fraction (0 to 1) with its configured threshold. Runs with no usable scores leave the monitor's previous breach state unchanged. Deploy the updated evaluation-job image before relying on pass-rate alerts; older jobs publish mean scores only by default.

Subscription event-type and environment edits are rechecked before every pending delivery attempt. Database failures before a send do not consume the destination's send-attempt budget or advance its backoff.

Monitor alert settings are shared by everyone managing a monitor. Configure them in the monitor's Alerting panel, selecting one or more organization destinations available to that agent. Editing requires `amp:monitor:update`; reading requires `amp:monitor:read`. The API is `GET`, `PUT`, and `DELETE .../monitors/{monitorName}/alerting`. There is no personal watch or console inbox API.

Each failure, completion, threshold breach, or recovery is sent to the selected destinations. Threshold state and delivery jobs commit together; bus retries reload those jobs after a crash. Before sending, the dispatcher rechecks that the alert still exists, still selects that destination, and the destination is still available to the agent. Delivery history remains under organization Events.

Migration 057 adds shared monitor alerts and durable alert payloads. No personal watch or console inbox tables are created. Select organization destinations explicitly when enabling monitor alerting.
