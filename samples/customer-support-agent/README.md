# Customer Support Agent - Deployment Guide

## Overview

The Customer Support Agent is an AI-powered customer service assistant that helps users with travel-related inquiries including flights, bookings, hotels, and car rentals. Built with LangGraph and FastAPI, this agent can search for information, make bookings, and provide comprehensive travel assistance.

**Based on**: [LangGraph Customer Support Tutorial](https://github.com/langchain-ai/langgraph/blob/main/docs/docs/tutorials/customer-support/customer-support.ipynb)

## Prerequisites

Before deploying this agent, ensure you have:

### Required API Keys

- **OpenAI API Key**: For GPT-powered conversations
- **Tavily API Key**: For web search capabilities

### Database

- **PostgreSQL Database**
- **Database Dump**: Apply the `db_backup.sql` dump to your PostgreSQL database before deployment

#### Setting up Database with Sample Data

1. Create your PostgreSQL database
2. Apply the database dump:

## Deployment Instructions

### Step 1: Access Agent Manager Platform

1. Navigate to the **Default** project
2. Click **"Add Agent"**

### Step 2: Configure Agent Details

Fill in the agent creation form with these exact values:

| Field                 | Value                                                   |
| --------------------- | ------------------------------------------------------- |
| **Display Name**      | `Customer Support Agent`                                |
| **Description**       | `AI-powered customer support agent for travel services` |
| **GitHub Repository** | `https://github.com/wso2/ai-agent-management-platform`  |
| **Branch**            | `main`                                                  |
| **App Path**          | `samples/customer-support-agent`                        |
| **Language**          | `Python`                                                |
| **Language Version**  | `3.11`                                                  |
| **Start Command**     | `python main.py`                                        |

### Step 3: Select Agent Interface

- Choose **"Chat Agent"** as the agent interface type

### Step 4: Configure Environment Variables

Add the following environment variables in the create form:

```env
OPENAI_API_KEY=<your-openai-api-key>
TAVILY_API_KEY=<your-tavily-api-key>
DATABASE_URL=<your-postgresql-connection-string>
```

### Step 5: Deploy the Agent

1. Review all configuration details
2. Click **"Deploy Agent"**
3. Wait for the build to complete (typically 2-5 minutes)

## Testing Your Agent

### Step 1: Access Development Environment

1. Navigate to your deployed agent
2. Click on the **"Development"** environment tab
3. Go to the **"Try Out"** section

### Step 2: Test Sample Interactions

Try these sample questions:

**Flight Inquiries:**

```json
{
  "thread_id": 1,
  "question": "What flights do I have booked?",
  "passenger_id": "3442 587242"
}
```

**Hotel Search:**

```json
{
  "thread_id": 2,
  "question": "Find me a hotel in Geneva for next week",
  "passenger_id": "3442 587242"
}
```

**General Travel Help:**

```json
{
  "thread_id": 3,
  "question": "I need to cancel my flight, can you help?",
  "passenger_id": "3442 587242"
}
```

### Step 3: Observe Traces

1. Click on the **"Observe"** tab
2. View traces
