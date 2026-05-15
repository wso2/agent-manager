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
2. Click **"Add Agent"**
3. Select **Platform-Hosted Agent** Card

### Step 2: Configure Agent Details

Fill in the agent creation form with these exact values:

| Field                 | Value                                                        |
| --------------------- | ------------------------------------------------------------ |
| **Display Name**      | `IT Helpdesk Agent`                                          |
| **Description**       | `AI-powered IT helpdesk agent for employee technical support` |
| **GitHub Repository** | `https://github.com/wso2/agent-manager`                      |
| **Branch**            | `main`                                                       |
| **App Path**          | `samples/it-helpdesk-agent`                                  |
| **Language**          | `Python`                                                     |
| **Language Version**  | `3.11`                                                       |
| **Start Command**     | `python main.py`                                             |
| **Port**              | `8000`                                                       |

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
Hi, I am bob.martinez@acmecorp.com. My email is not syncing — is something wrong?
```

### Step 3: Observe Traces

1. Click on the **"Observability"** tab on left navigation and select **Traces**
2. View traces

## Testing Guardrails

After configuring an LLM provider with guardrails enabled, test the following query to verify PII detection:

**PII in input (user exposes a password):**

```text
My password is P@ssw0rd123! and it's not working. My email is alice.chen@acmecorp.com.
```

Without guardrails, the agent accepts this message and the password gets logged in traces. With a PII detection guardrail configured, the platform should intercept and block or redact the sensitive content before it reaches the agent.
