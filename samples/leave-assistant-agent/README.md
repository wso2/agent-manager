# Leave Assistant Agent (Ballerina) - Deployment Guide

## Overview

An HR leave-request assistant built with [Ballerina](https://ballerina.io)'s `ai` module and OpenAI. Employees can check their leave balance, file annual, sick, or unpaid leave requests, list their requests, and cancel a pending or approved one. A manager can approve or reject a request that's waiting for a decision.

The agent enforces real approval logic itself, not just record-keeping:

- **Annual leave of 3 days or fewer, and all sick leave**, are approved automatically as long as the employee has enough balance. The balance is deducted immediately.
- **Longer annual leave, and every unpaid request**, stay `pending` until `approveLeaveRequest` or `rejectLeaveRequest` is called.
- **Cancelling or rejecting** a request refunds any annual or sick balance it had deducted.
- Requesting more days than an employee has left is rejected outright with a clear error, not silently approved.

The service is a `ballerina/ai` `ai:Listener` chat service exposing `POST /leave-assistant/chat` on port `9090`.

This is the reference sample for Agent Manager's **Ballerina buildpack**. Unlike the Python samples in this repository, it's built directly from source with no Dockerfile, and, because Ballerina buildpacks don't need one, no language version or start command to configure.

## Sample Employees

Two employees are seeded in memory so you can exercise both the happy path and the balance-error path right away:

| Employee id | Name | Annual leave balance | Sick leave balance |
| --- | --- | --- | --- |
| `E-3001` | Priya Fernando | 14 days | 7 days |
| `E-3002` | Kasun Silva | 3 days | 7 days |

## Prerequisites

### Required API Keys

- **OpenAI API Key**: for model inference (the agent uses `gpt-4o`)

## Deployment Instructions

### Step 1: Access Agent Manager

1. Navigate to the **Default** project
2. Click **"Add Agent"**
3. Select the **Platform-Hosted Agent** card
4. Pick **Source Code** as the source type

### Step 2: Configure Agent Details

Fill in the agent creation form with these values:

| Field | Value |
| --- | --- |
| **Display Name** | `Leave Assistant` |
| **Description** | `Ballerina employee leave-request assistant` |
| **GitHub Repository** | `https://github.com/wso2/agent-manager` |
| **Branch** | `main` |
| **App Path** | `samples/leave-assistant-agent` |
| **Language** | `Ballerina` |

Ballerina is the one buildpack that doesn't ask for a **Language Version** or **Start Command**. The platform builds and runs `main.bal` directly, and the fields don't appear once you select it.

### Step 3: Select Agent Interface

- Choose **"Chat Agent"** as the agent interface type
- Set **Port** to `9090` and **Base Path** to `/leave-assistant`

For a Ballerina agent, **Chat Agent** means an `ai:Listener` service: the platform calls `POST <base path>/chat` with `{message, sessionId}` and expects `{message}` back, which is what **Try It** sends.

### Step 4: Configure Environment Variables

Ballerina reads `configurable` values from environment variables named `BAL_CONFIG_VAR_<NAME>`, where `<NAME>` is the configurable's identifier, uppercased. This agent declares one configurable, `openAiApiKey`, so set:

```env
BAL_CONFIG_VAR_OPENAIAPIKEY=<your-openai-api-key>
```

This is different from every Python sample in this repository, which read a plain `OPENAI_API_KEY`. It's the same convention the [AMP instrumentation guide](https://github.com/wso2/docs-agent-platform/blob/main/docs/guides/amp-instrumentation.mdx) uses for `ballerinax/amp`'s own configurables, just applied to this agent's own key instead.

### Step 5: Deploy the Agent

1. Review all configuration details
2. Click **"Deploy"**
3. Wait for the build to complete

## Testing Your Agent

### Step 1: Navigate to Chat Interface

Click on the **"Try It"** section on the left navigation.

### Step 2: Test Sample Interactions

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

### Step 3: Observe Traces

1. Click on the **"Observability"** tab on the left navigation and select **Traces**
2. Open a trace to see which tools were called for each message. `getCurrentDate` should appear before `requestLeave` whenever the message uses a relative date like "tomorrow" or "next Monday"

## Run Locally

```bash
cd samples/leave-assistant-agent
echo 'openAiApiKey = "<your-openai-api-key>"' > Config.toml
bal run    # serves on http://localhost:9090/leave-assistant
```

```bash
curl -s localhost:9090/leave-assistant/chat \
  -H 'content-type: application/json' \
  -d '{"sessionId": "s1", "message": "What is my leave balance? My employee id is E-3001"}'
```

Expect back `{"message": "..."}`.

`Config.toml` is git-ignored. Never commit real credentials to it.

## Notes

- **The chat service uses `ballerina/ai`'s own `ai:Listener`, `ChatReqMessage` and `ChatRespMessage`.** Agent Manager calls a Ballerina Chat Agent with that contract (`{message, sessionId}` in, `{message}` out), on the port and base path you set when creating the agent. The listener uses Ballerina's default HTTP listener, port `9090`.
- Leave request ids are returned by `requestLeave` and `listMyLeaveRequests`. The system prompt tells the agent to reuse an id from an earlier reply rather than ask the user for one, which is why the test sequence above refers to a request by description and still works. The agent resolves it to an id itself via `listMyLeaveRequests`.
- `approveLeaveRequest` and `rejectLeaveRequest` aren't gated behind any real authorization in this sample. Anyone in the chat can call them. That's deliberate: the point here is the Ballerina buildpack and the approval *logic*, not building a role-based access model. A production version would check the caller's role before allowing either.
- The in-memory employee and leave-request stores reset on every restart. There's no database. That's deliberate too: the point of this sample is the Ballerina buildpack and the `ai:Agent` / `@ai:AgentTool` pattern, not persistence.
- `ballerinax/amp` is imported for its side effect only (`import ballerinax/amp as _;`). It wires up AMP's OpenTelemetry auto-instrumentation with no code changes beyond the import.
