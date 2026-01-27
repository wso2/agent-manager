# Finance Insight UI - Implementation & Backend Integration Guide

This document summarizes the current UI, how the frontend is wired, and how it
connects to the backend for a stateless, scenario-based report flow.

## What was implemented

- Next.js (App Router + TypeScript) UI in `src/ui/`.
- Scenario-based report layout with a top bar and "New request" button.
- Dark/light mode toggle with smooth transitions.
- Real typing input (textarea) with Enter-to-send.
- Settings page for API URL + API key (stored locally).

## Key UI routes

- `/` main report UI
- `/settings` API authentication + connection test

## How the frontend talks to your backend

Frontend integration lives in `src/ui/lib/api.ts`:

- `GET /health` used by the Settings page test button.
- `GET /config` used by the Settings page to show service status.
- `POST /chat/async` used to submit a report request (polls for status).

### Request headers

If an API key is set in `/settings`, the UI sends:

- `Authorization: Bearer <apiKey>`
- `X-API-Key: <apiKey>`

### Result format expected by the UI

The UI expects:

- `{ result: { report: "text" } }`

### Environment option

You can set a default backend URL in `src/ui/.env.local`:

```
NEXT_PUBLIC_API_BASE_URL=http://localhost:5000
```

The Settings page overrides this per browser (stored in `localStorage`).

## CrewAI integration notes

- Run the CrewAI workflow inside `/chat/async`.
- Return a single `report` string in the final result.

## Security notes

- API keys stored in localStorage are fine for local dev only.
- For production, use server-side auth (sessions/JWT) and do not expose keys.
- Use HTTPS and restrict CORS origins to your frontend domain.
