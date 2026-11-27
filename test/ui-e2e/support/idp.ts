import { request } from '@playwright/test';
import { idpClientId, idpClientSecret, thunderURL } from './env';

// Every scope the API checks, requested in full: ThunderID puts only the
// requested scopes it allows into a client_credentials token, and a token
// without them is refused on every guarded route. The same list as the Go e2e
// suite's (test/e2e/framework/auth.go); change both.
const ampScopes = [
  'amp:agent:build amp:agent:create amp:agent:delete amp:agent:read amp:agent:suspend',
  'amp:agent:token-manage amp:agent:update',
  'amp:environment:create amp:environment:delete amp:environment:read amp:environment:update',
  'amp:group:create amp:group:delete amp:group:read amp:group:update',
  'amp:events:read amp:events:manage',
  'amp:org:assign-role amp:org:invite-member amp:org:manage-idp amp:org:manage-service-account',
  'amp:org:modify-settings amp:org:remove-member amp:org:view',
  'amp:project:create amp:project:delete amp:project:read amp:project:update',
  'amp:role:create amp:role:delete amp:role:read amp:role:update',
  'amp:profile:read amp:profile:update-attributes',
  'amp:ai-gateway:gateways.create amp:ai-gateway:gateways.read amp:ai-gateway:gateways.update',
  'amp:ai-gateway:gateways.delete amp:ai-gateway:gateways.token-manage',
  'amp:ai-gateway:llm-providers.create amp:ai-gateway:llm-providers.read amp:ai-gateway:llm-providers.update',
  'amp:ai-gateway:llm-providers.delete amp:ai-gateway:llm-providers.configure-guardrail',
  'amp:ai-gateway:llm-providers.connect amp:ai-gateway:llm-providers.deploy amp:ai-gateway:llm-providers.api-key-manage',
  'amp:ai-gateway:llm-provider-templates.create amp:ai-gateway:llm-provider-templates.read',
  'amp:ai-gateway:llm-provider-templates.update amp:ai-gateway:llm-provider-templates.delete',
  'amp:ai-gateway:llm-proxies.create amp:ai-gateway:llm-proxies.read amp:ai-gateway:llm-proxies.update',
  'amp:ai-gateway:llm-proxies.delete amp:ai-gateway:llm-proxies.deploy amp:ai-gateway:llm-proxies.api-key-manage',
  'amp:ai-gateway:mcp-servers.create amp:ai-gateway:mcp-servers.read amp:ai-gateway:mcp-servers.update',
  'amp:ai-gateway:mcp-servers.delete amp:ai-gateway:mcp-servers.configure-guardrail',
  'amp:ai-gateway:mcp-servers.connect amp:ai-gateway:mcp-servers.api-key-manage',
  'amp:ai-gateway:scopes.create amp:ai-gateway:scopes.read amp:ai-gateway:scopes.update amp:ai-gateway:scopes.delete',
  'amp:ai-gateway:catalog.read',
  'amp:ai-gateway:agent-identities.create amp:ai-gateway:agent-identities.read',
  'amp:ai-gateway:agent-identities.update amp:ai-gateway:agent-identities.delete',
  'amp:ai-gateway:agent-api-keys.manage amp:ai-gateway:deploy.non-production amp:ai-gateway:deploy.production',
  'amp:observability:traces-read',
  'amp:ai-hub:plan-read amp:ai-hub:plan-create amp:ai-hub:plan-update amp:ai-hub:plan-delete',
  'amp:ai-hub:webhook-secret-read amp:ai-hub:webhook-secret-create',
  'amp:ai-hub:webhook-secret-rotate amp:ai-hub:webhook-secret-delete',
].join(' ');

/** A token for the API's machine client, from ThunderID's client_credentials grant. */
export async function idpToken(): Promise<string> {
  const ctx = await request.newContext();
  try {
    const res = await ctx.post(`${thunderURL}/oauth2/token`, {
      headers: {
        Authorization: `Basic ${Buffer.from(`${idpClientId}:${idpClientSecret}`).toString('base64')}`,
      },
      form: { grant_type: 'client_credentials', scope: ampScopes },
    });
    if (!res.ok()) {
      throw new Error(`ThunderID refused the API client's token request (${res.status()}): ${await res.text()}`);
    }
    return (await res.json()).access_token;
  } finally {
    await ctx.dispose();
  }
}
