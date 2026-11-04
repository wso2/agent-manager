# Insurance Support Agent — Web Client

This is the invoking client for the insurance support agent: a React and
TypeScript chat window that runs in the browser. The agent itself has no
authentication logic and must not gain any — the gateway in front of it
validates the caller's token. This client's only job on the auth side is to
obtain a token and attach it as `Authorization: Bearer` on every request.

## Prerequisites

- Node 22.x from 22.23.4, 24.x from 24.22.1, or 26.12.1 and later (`node -v`). Node 20, 23 and 25 are not supported.
- An agent to talk to — running locally on port 8000, or deployed and
  reachable behind the gateway

## Quick start

```bash
cd samples/insurance-support-agent/web
npm install
cp .env.example .env
npm run dev
```

Open <http://localhost:5173>. The browser needs CORS headers from whatever it
calls: against a **deployed** agent, add `http://localhost:5173` to the agent's
CORS settings in Agent Manager; against an agent running **locally**, start it
with `CORS_ALLOW_ORIGINS=http://localhost:5173` instead. See
[the root README](../README.md#cors).

## Environment variables

| Variable               | Required              | Default                      | Purpose                                                                  |
| ---------------------- | --------------------- | ---------------------------- | ------------------------------------------------------------------------ |
| `VITE_AGENT_URL`       | no                    | `http://localhost:8000/chat` | Base or full `/chat` URL; `/chat` is appended when it isn't already there |
| `VITE_AUTH_MODE`       | no                    | `none`                       | `none` or `oauth`                                                        |
| `VITE_OAUTH_ISSUER`    | required when `oauth` | —                            | Issuer base URL; `/.well-known/openid-configuration` is appended         |
| `VITE_OAUTH_CLIENT_ID` | required when `oauth` | —                            | Public client ID                                                         |
| `VITE_OAUTH_SCOPES`    | no                    | `openid profile email`       | Requested scopes                                                         |
| `VITE_COMPANY_NAME`    | no                    | `O2 Insurance`               | Used in the chat header                                                  |

## Pointing at a deployed agent

Set `VITE_AGENT_URL` in `.env`, or append `?agent=<url>` to the page URL — the
value persists to `localStorage` until you load the page with `?agent=reset`.
The override only takes effect in `none` mode: in `oauth` mode it is ignored
entirely (not read, not stored, and any value stored from an earlier `none`
visit is not used either), because honouring it would send the signed-in
user's access token to whatever origin the URL names. Either way, this must
be the **gateway** URL. Pointing straight at the agent bypasses the thing
being secured.

## The two modes

In `none` mode, the client sends no token — it posts straight to the agent,
and the header shows a `● no login` badge. This only works against an
unprotected agent.

In `oauth` mode, the client shows a sign-in screen and performs an
authorization-code flow with PKCE as a public client. The access token is kept
in `sessionStorage`, and once signed in the header shows a **Sign out** button.
`VITE_AUTH_MODE` is trimmed and lower-cased before matching, so `OAUTH`,
`OAuth` and ` oauth ` all resolve to `oauth`. If it's set
but the issuer or client id is missing, or if it's set to something that
isn't `none` or `oauth`, the client shows a configuration screen naming the
problem rather than silently falling back to no-login.

## The chat window

Agent replies are rendered as Markdown — headings, bold text, lists and tables
come through formatted rather than as raw `**` and `###`. **New chat** in the
header clears the transcript and starts a fresh `session_id`, so the agent
begins with no history; a reply still in flight from the old conversation is
discarded rather than appended to the new one.

## Register the callback URL

Register `http://localhost:5173/` as the callback URL with your identity
provider. It's derived from the page origin, not configured in `.env` — so if
you change the dev server port, update the registration to match.

## The Thunder walkthrough

Every installation ships ThunderID, already registered with the gateway as
`ThunderKeyManager`, so there is no identity provider to add.

1. **Create an OAuth application in Thunder.** It must be a public client with
   PKCE — grant type `authorization_code`, token endpoint auth method `none`,
   and callback URL `http://localhost:5173/`. Note the client ID and the
   Thunder issuer URL.
2. **Enable OAuth on the agent.** Open the agent, click **Deploy** →
   **Configure & Deploy**, and under **Endpoint Authentication** select
   **OAuth**, then choose `ThunderKeyManager`.
3. **Run the client with the provider configured:**

   ```bash
   VITE_AUTH_MODE=oauth VITE_OAUTH_ISSUER=<thunder-issuer-url> VITE_OAUTH_CLIENT_ID=<client-id-from-step-1> npm run dev
   ```

   Or set the same values in `.env` and run `npm run dev` as usual.

## Any other provider (Asgardeo, Okta, Entra)

The client is discovery-driven — it reads the issuer's
`.well-known/openid-configuration` and uses whatever endpoints it advertises.

1. Create the equivalent single-page-application public client with your
   provider, with PKCE and callback `http://localhost:5173/`.
2. Register the provider once in the console under **Gateways → Identity
   Providers**: paste the issuer URL and let discovery populate the issuer and
   JWKS URI.
3. Select that provider instead of `ThunderKeyManager` when enabling OAuth on
   the agent, and set `VITE_OAUTH_ISSUER` and `VITE_OAUTH_CLIENT_ID` to its
   values.

Nothing else changes.

## Confirming enforcement

```bash
curl -i -X POST https://<your-agent-url>/chat \
  -H 'Content-Type: application/json' \
  -d '{"session_id":"s1","message":"hello"}'
```

Expected `401`.

## Known limits

Conversation history lives in process memory keyed on the caller-supplied
`session_id`. The gateway authenticates the caller, but the agent never reads
the token, so it cannot tell one signed-in customer from another — anyone
holding a valid token who knows a `session_id` can resume that conversation. A
real deployment should key sessions on the authenticated subject and move them
out of process memory. The store is also capped at 500 sessions and evicts the
oldest first, which is enough for a demo and nothing more.

## Use your provider's SDK instead

The auth layer is a single interface:

```ts
export interface Auth {
  mode: string;
  status: "loading" | "signed-out" | "signed-in" | "error";
  user?: { name?: string };
  signIn(): void;
  signOut(): void;
  getAccessToken(): Promise<string | null>;
  error?: string;
}
```

To use a different provider's SDK, add a file under `src/lib/auth/` that
returns this shape. `App.tsx` only ever calls `useAuth()` (`api.ts` takes the
token as a plain argument), so the rest of the chat UI doesn't change. Wiring
in a genuinely new mode — one `AuthMode` doesn't already cover — is still
three edits:

1. Add the mode to `AuthMode` and `resolveMode()` in `src/lib/config.ts`.
2. Select it in `src/lib/auth/index.tsx`.
3. Mount the vendor's provider in `src/main.tsx`.

<details><summary>Adapter: Auth0 (<code>@auth0/auth0-react</code>)</summary>

```tsx
import { useAuth0 } from "@auth0/auth0-react";

import type { Auth } from "./types";

export function useAuth0Adapter(): Auth {
  const { isLoading, isAuthenticated, user, loginWithRedirect, logout, getAccessTokenSilently } =
    useAuth0();

  return {
    mode: "auth0",
    status: isLoading ? "loading" : isAuthenticated ? "signed-in" : "signed-out",
    user: user ? { name: user.given_name ?? user.name } : undefined,
    signIn: () => void loginWithRedirect(),
    signOut: () => logout({ logoutParams: { returnTo: window.location.origin } }),
    getAccessToken: () => (isAuthenticated ? getAccessTokenSilently() : Promise.resolve(null)),
  };
}
```

And the provider mount in `main.tsx`:

```tsx
<Auth0Provider
  domain={import.meta.env.VITE_AUTH0_DOMAIN}
  clientId={import.meta.env.VITE_AUTH0_CLIENT_ID}
  authorizationParams={{
    redirect_uri: window.location.origin,
    audience: import.meta.env.VITE_AUTH0_AUDIENCE,
  }}
>
  <AuthProvider>
    <App />
  </AuthProvider>
</Auth0Provider>
```

`authorizationParams.audience` must be the API identifier registered in Auth0.
Omit it and `getAccessTokenSilently()` returns an opaque token rather than a
JWT, the gateway rejects it, and the failure reads like an auth bug when it is
a configuration one.

</details>

<details><summary>Adapter: ThunderID (<code>@thunderid/react</code>)</summary>

The published SDK (`@thunderid/react` 1.0.6) exposes `useThunderID()` with a
`getAccessToken(): Promise<string>` accessor alongside `isSignedIn`,
`isLoading`, `user`, `signIn` and `signOut` — so this adapter maps directly
onto the same `Auth` shape as the others. The SDK types `user` as `any` and
its own docs are inconsistent about its claim shape, so confirm the actual
fields against the version you install rather than trusting the optional
chain below blindly.

```tsx
import { useThunderID } from "@thunderid/react";

import type { Auth } from "./types";

export function useThunderIDAdapter(): Auth {
  const { isLoading, isSignedIn, user, signIn, signOut, getAccessToken } = useThunderID();

  return {
    mode: "thunderid",
    status: isLoading ? "loading" : isSignedIn ? "signed-in" : "signed-out",
    user: user ? { name: user.name?.givenName ?? user.givenName ?? user.displayName } : undefined,
    signIn: () => void signIn(),
    signOut: () => void signOut(),
    getAccessToken: () => (isSignedIn ? getAccessToken() : Promise.resolve(null)),
  };
}
```

And the provider mount in `main.tsx`:

```tsx
<ThunderIDProvider
  baseUrl={import.meta.env.VITE_THUNDERID_BASE_URL}
  clientId={import.meta.env.VITE_THUNDERID_CLIENT_ID}
>
  <AuthProvider>
    <App />
  </AuthProvider>
</ThunderIDProvider>
```

</details>
