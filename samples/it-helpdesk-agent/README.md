# IT Helpdesk Agent - Deployment Guide

## Overview

The IT Helpdesk Agent is an AI-powered L1 IT support assistant that helps employees with password resets, software access requests, ticket management, and system status inquiries. Built with LangGraph and FastAPI, this agent enforces identity verification, respects admin account restrictions, detects duplicate tickets, and escalates complex issues to L2 support.

## Prerequisites

Before deploying this agent, ensure you have:

### Required API Keys

- **OpenAI API Key**: For GPT-powered conversations


## Deployment Instructions

### Step 1: Access Agent Manager

1. Navigate to the **Default** project
2. Select **Platform-Hosted Agent** Card
3. Pick **Source Code** as the source type of the agent

### Step 2: Configure Agent Details

Fill in the agent creation form with these exact values:

| Field                 | Value                                                        |
| --------------------- | ------------------------------------------------------------ |
| **Display Name**      | `IT Helpdesk Agent`                                          |
| **Description**       | `AI-powered IT helpdesk agent for employee technical support` |
| **GitHub Repository** | `https://github.com/wso2/agent-manager`                      |
| **Branch**            | `main`                                                       |
| **App Path**          | `/samples/it-helpdesk-agent`                                  |
| **Language**          | `Python`                                                     |
| **Language Version**  | `3.11`                                                       |
| **Start Command**     | `python main.py`                                             |

### Step 3: Select Agent Interface

- Choose **"Chat Agent"** as the agent interface type

### Step 4: Configure Environment Variables

Add the following environment variables in the create form:

```env
OPENAI_API_KEY=<your-openai-api-key>
```

### Step 5: Deploy the Agent

1. Review all configuration details
2. Click **"Deploy"**
3. Wait for the build to complete (typically 6-10 minutes)

## Testing Your Agent

### Step 1: Navigate to Chat Interface

Click on the **"Try It"** section on the left navigation.

### Step 2: Test Sample Interactions

Try these sample queries in the chat interface. Each query exercises a different tool chain — visible in traces.

**Password reset (happy path — multi-step tool chain):**

```text
Hi, I am alice.chen@acmecorp.com, employee ID E-1001. I forgot my password and need a reset.
```

**Password reset blocked (admin account → escalation):**

```text
Hi, david.kim@acmecorp.com here, employee ID E-1004. I need my password reset urgently.
```

**Known outage detection (agent checks status, no ticket created):**

```text
Hi, I am bob.martinez@acmecorp.com, employee ID E-1002. My email is not syncing — is something wrong?
```

### Step 3: Observe Traces

1. Navigate to **Observability** > **Traces** on the left navigation
2. Click on a trace to view the tool call chain for each interaction

## Testing Guardrails

After configuring an LLM provider with the **PII Masking Regex** guardrail (with **SSN** detection enabled):

1. In the LLM provider's **Environment Variables References**, rename the variables to match the agent's code:
   - `Base URL of the LLM provider` → `LLM_PROVIDER_URL`
   - `API Key for authentication` → `LLM_PROVIDER_KEY`
2. Add the following environment variable to the agent:
   ```env
   USE_LLM_PROVIDER=true
   ```

Then test the following query:

**PII in input (user exposes their SSN):**

```text
Hi, I am alice.chen@acmecorp.com, employee ID E-1001. My SSN is 123-45-6789. I need to update my payroll info, can you help?
```

Without guardrails, the agent accepts this message and the SSN gets logged in traces. With the SSN guardrail enabled, the platform redacts the SSN before it reaches the agent.
