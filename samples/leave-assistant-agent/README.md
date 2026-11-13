# Leave Assistant Agent (Ballerina)

## Overview

An HR leave-request assistant built with [Ballerina](https://ballerina.io)'s `ai` module and OpenAI. Employees can check their leave balance, file annual, sick, or unpaid leave requests, list their requests, and cancel a pending or approved one. A manager can approve or reject a request that's waiting for a decision.

The agent enforces real approval logic itself, not just record-keeping:

- **Annual leave of 3 days or fewer, and all sick leave**, are approved automatically as long as the employee has enough balance. The balance is deducted immediately.
- **Longer annual leave, and every unpaid request**, stay `pending` until `approveLeaveRequest` or `rejectLeaveRequest` is called.
- **Cancelling or rejecting** a request refunds any annual or sick balance it had deducted.
- Requesting more days than an employee has left is rejected outright with a clear error, not silently approved.

The service exposes `POST /chat` on port `8000`.

This is the reference Ballerina sample. Instead of `amp-instrument`, it traces through the `ballerinax/amp` module: one side-effect import plus `observabilityIncluded = true` in `Ballerina.toml`.

## Sample Employees

Two employees are seeded in memory so you can exercise both the happy path and the balance-error path right away:

| Employee id | Name | Annual leave balance | Sick leave balance |
| --- | --- | --- | --- |
| `E-3001` | Priya Fernando | 14 days | 7 days |
| `E-3002` | Kasun Silva | 3 days | 7 days |

## Prerequisites

- [Ballerina](https://ballerina.io/downloads/) `2201.13.4`, the distribution `Ballerina.toml` declares.
- An **OpenAI API key** for model inference (the agent uses `gpt-4o`).
- Agent Manager running. From the repository root, `make up` starts it (see [`deployments/standalone`](../../deployments/standalone/README.md)).

## Register it in Agent Manager

You run the agent yourself; Agent Manager registers it and issues the token its traces are sent with.

1. Sign in to the console at <http://localhost:3000> as `admin` (the password is `AMP_ADMIN_PASSWORD` in `deployments/standalone/.env`).
2. Open the **Default** project, click **Add Agent**, and register an agent named `Leave Assistant`.
3. In the **Set up agent** panel that opens, choose **Ballerina** and version **2201.13.x**, pick a token duration and click **Generate**. Copy the endpoint and API key it shows.

## Run it

Ballerina reads `configurable` values from environment variables named `BAL_CONFIG_VAR_<NAME>`, where `<NAME>` is the configurable's identifier, uppercased, prefixed with its module for imported modules. This agent declares `openAiApiKey`; `ballerinax/amp` declares the trace endpoint and key:

```bash
cd samples/leave-assistant-agent

export BAL_CONFIG_VAR_OPENAIAPIKEY="<your-openai-api-key>"
export BAL_CONFIG_VAR_BALLERINAX_AMP_OTELENDPOINT="http://default-default.gateway.localhost:19080/otel"
export BAL_CONFIG_VAR_BALLERINAX_AMP_APIKEY="<key-from-the-console>"

bal run    # serves on http://localhost:8000
```

The Python samples in this repository read a plain `OPENAI_API_KEY` instead. You can also put the values in a git-ignored `Config.toml`; never commit real credentials to it.

## Try it

```bash
curl -s localhost:8000/chat \
  -H 'content-type: application/json' \
  -d '{"session_id": "s1", "message": "What is my leave balance? My employee id is E-3001"}'
```

Expect back `{"response": "..."}`. See the note below on why the field names are `session_id`/`response`, not Ballerina's own `sessionId`/`message`.

### Sample interactions

Try these in order. Together they exercise every rule the agent enforces, not just the tool calls:

**Check a balance:**

```text
What is my leave balance? My employee id is E-3001
```

**Auto-approved (short annual leave, within balance):**

```text
I am E-3001, request 2 days annual leave starting tomorrow, family event
```

**Needs manager approval (over the 3-day threshold):**

```text
I am E-3001, request 5 days annual leave starting next Monday, taking a trip
```

**Balance error, not a partial approval:**

```text
I am E-3002, request 4 days annual leave starting Friday
```

E-3002 only has 3 annual days, so this should come back as a clear rejection with the actual balance stated, not a request filed for fewer days than asked.

**List and cancel:**

```text
List my leave requests, employee id E-3001
```

```text
Cancel the family event leave request for E-3001
```

Check E-3001's balance again after cancelling. The 2 days should be back, confirming the refund fires.

**Manager approving the pending request** (a separate conversation, standing in for a manager's side of the flow):

```text
Approve the pending 5-day leave request for E-3001
```

### Observe the traces

Open the agent in the console and go to **Observability → Traces**. Open a trace to see which tools were called for each message. `getCurrentDate` should appear before `requestLeave` whenever the message uses a relative date like "tomorrow" or "next Monday"

## Notes

- **The request/response types are hand-written, not `ballerina/ai`'s own `ChatReqMessage`/`ChatRespMessage`.** Those module types use `sessionId`/`message`. Agent Manager's Chat Agent contract sends `session_id` and expects `response` back, matching every Python sample here, plus an unused `context` field. Binding straight to `ai:ChatReqMessage` also requires `ai:Listener`, whose `ChatService` contract locks you into that exact camelCase shape with no way to override it. This sample uses a plain `http:Listener` with its own open `ChatRequest`/closed `ChatResponse` types instead. If you're adapting this pattern for your own Ballerina agent, keep that distinction, or you'll hit the same 400s (`undefined field 'session_id'`, then `undefined field 'context'`) the moment a real chat client calls it instead of a hand-written curl test.
- Leave request ids are returned by `requestLeave` and `listMyLeaveRequests`. The system prompt tells the agent to reuse an id from an earlier reply rather than ask the user for one, which is why the test sequence above refers to a request by description and still works. The agent resolves it to an id itself via `listMyLeaveRequests`.
- `approveLeaveRequest` and `rejectLeaveRequest` aren't gated behind any real authorization in this sample. Anyone in the chat can call them. That's deliberate: the point here is the Ballerina agent and the approval *logic*, not building a role-based access model. A production version would check the caller's role before allowing either.
- The in-memory employee and leave-request stores reset on every restart. There's no database. That's deliberate too: the point of this sample is the `ai:Agent` / `@ai:AgentTool` pattern, not persistence.
- `ballerinax/amp` is imported for its side effect only (`import ballerinax/amp as _;`). It wires up AMP's OpenTelemetry auto-instrumentation with no code changes beyond the import.
