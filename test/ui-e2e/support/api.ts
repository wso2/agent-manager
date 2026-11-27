import { APIRequestContext, expect, request } from '@playwright/test';
import { adminPassword, adminUser, authMode, baseURL } from './env';
import { idpToken } from './idp';

// A thin API client the specs use to set up their own resources and to read
// back what the UI did, through a second channel. With file sign-in it signs in
// as the admin; with an identity provider it is the API's machine client.
export class AmpApi {
  private constructor(readonly ctx: APIRequestContext, readonly token: string) {}

  static async signIn(): Promise<AmpApi> {
    const token = authMode === 'idp' ? await idpToken() : await fileToken();
    const ctx = await request.newContext({
      baseURL,
      extraHTTPHeaders: { Authorization: `Bearer ${token}` },
    });
    return new AmpApi(ctx, token);
  }

  /** Path of the identities API; the organization comes from the token. */
  get identities(): string {
    return '/api/v1/identities';
  }

  /** Path of the projects collection, or of one project. */
  projectPath(name?: string): string {
    return name ? `/api/v1/projects/${encodeURIComponent(name)}` : '/api/v1/projects';
  }

  /** Path of one agent, its project in the query. */
  agentPath(project: string, name: string): string {
    return `/api/v1/agents/${encodeURIComponent(name)}?project=${encodeURIComponent(project)}`;
  }

  /** Path of one AI Gateway gateway, by name or UUID. */
  gatewayPath(ref: string): string {
    return `/m/ai-gateway/gateways/${encodeURIComponent(ref)}`;
  }

  async createProject(name: string, displayName = name): Promise<void> {
    const res = await this.ctx.post(this.projectPath(), {
      data: {
        apiVersion: 'ai-control-plane.choreolabs.com/v1',
        kind: 'Project',
        metadata: { name },
        spec: { displayName },
      },
    });
    expect(res.status(), await res.text()).toBe(202);
  }

  async getProject(name: string): Promise<{ status: number; body: any }> {
    const res = await this.ctx.get(this.projectPath(name));
    return { status: res.status(), body: res.ok() ? await res.json() : undefined };
  }

  async deleteProject(name: string): Promise<void> {
    // Agents first: a project with agents may refuse to go.
    const agents = await this.ctx.get('/api/v1/agents', { params: { project: name, limit: 500 } });
    if (agents.ok()) {
      const body = await agents.json();
      for (const a of body.list ?? []) {
        await this.ctx.delete(this.agentPath(name, a.metadata.name));
      }
    }
    // Agent deletion finishes in the background, and a project with agents
    // left is refused: delete again until the project is gone.
    await expect
      .poll(
        async () => {
          await this.ctx.delete(this.projectPath(name));
          return (await this.ctx.get(this.projectPath(name))).status();
        },
        { timeout: 60_000, intervals: [1_000, 2_000, 5_000] },
      )
      .toBe(404);
  }

  /** Removes a user found by username, if it exists. */
  async deleteUser(username: string): Promise<void> {
    const res = await this.ctx.get(`${this.identities}/users`, { params: { limit: 200 } });
    const users: any[] = res.ok() ? (await res.json()).users ?? [] : [];
    const user = users.find((u) => u.attributes?.username === username);
    if (user) await this.ctx.delete(`${this.identities}/users/${user.id}`);
  }

  async createExternalAgent(project: string, name: string, displayName = name): Promise<void> {
    const res = await this.ctx.post('/api/v1/agents', {
      data: {
        apiVersion: 'ai-control-plane.choreolabs.com/v1',
        kind: 'Agent',
        metadata: { name, project },
        spec: {
          displayName,
          provisioning: { type: 'external' },
          agentType: { type: 'external-agent-api', subType: 'custom-api' },
        },
      },
    });
    expect(res.status(), await res.text()).toBe(202);
  }

  async getAgent(project: string, name: string): Promise<{ status: number; body: any }> {
    const res = await this.ctx.get(this.agentPath(project, name));
    return { status: res.status(), body: res.ok() ? await res.json() : undefined };
  }

  async get(path: string): Promise<{ status: number; body: any }> {
    const res = await this.ctx.get(path);
    let body: any;
    try {
      body = await res.json();
    } catch {
      body = undefined;
    }
    return { status: res.status(), body };
  }

  async delete(path: string): Promise<number> {
    return (await this.ctx.delete(path)).status();
  }

  async post(path: string, data: unknown): Promise<{ status: number; body: any }> {
    const res = await this.ctx.post(path, { data });
    return { status: res.status(), body: await res.json().catch(() => undefined) };
  }

  /** Subscription plans (the AI Hub module): served while the module is installed. */
  readonly plans = '/m/ai-hub/subscription-plans';

  /** Creates a plan through the API, as a document; metadata.name is its handle. */
  async createPlan(handle: string, spec: Record<string, unknown>): Promise<void> {
    const res = await this.post(this.plans, {
      apiVersion: 'ai-control-plane.choreolabs.com/v1',
      kind: 'SubscriptionPlan',
      metadata: { name: handle },
      spec,
    });
    expect(res.status, JSON.stringify(res.body)).toBe(201);
  }

  /**
   * An active gateway with the environment's UUID, or undefined when none is active. Every gateway
   * serves the organization's one environment; a proxy names the gateway.
   */
  async activeGateway(envName = 'default'): Promise<{ envUuid: string; gatewayId: string } | undefined> {
    const env = await this.get(`/api/v1/environments/${encodeURIComponent(envName)}`);
    if (env.status !== 200) return undefined;
    const { body } = await this.get('/m/ai-gateway/gateways');
    const gw = (body?.list ?? []).find((g: any) => g.status?.state === 'ACTIVE');
    return gw ? { envUuid: env.body.id, gatewayId: gw.metadata.uid } : undefined;
  }

  /** Creates an org MCP proxy with one endpoint on the gateway, offering the plans. */
  async createMcpProxy(
    handle: string,
    target: { envUuid: string; gatewayId: string },
    plans: string[],
  ): Promise<void> {
    const res = await this.post('/m/ai-gateway/mcp-proxies', {
      apiVersion: 'ai-control-plane.choreolabs.com/v1',
      kind: 'McpProxy',
      metadata: { name: handle },
      spec: {
        displayName: `UI e2e ${handle}`,
        version: 'v1.0',
        context: `/${handle}`,
        endpoints: [{
          id: 'primary',
          // An address, not a name: the server checks that the upstream host resolves. Nothing calls it.
          upstream: { main: { url: 'https://1.1.1.1/mcp' } },
          environments: [{ environmentUuid: target.envUuid, gatewayId: target.gatewayId }],
        }],
        subscriptionPlans: plans,
      },
    });
    expect(res.status, JSON.stringify(res.body)).toBe(201);
  }

  async dispose(): Promise<void> {
    await this.ctx.dispose();
  }
}

/** The admin's token from file sign-in, through the API. */
async function fileToken(): Promise<string> {
  const anon = await request.newContext({ baseURL });
  const res = await anon.post('/auth/login', {
    data: { username: adminUser, password: adminPassword() },
  });
  expect(res.status(), 'admin sign-in through the API').toBe(200);
  const { access_token: token } = await res.json();
  await anon.dispose();
  return token;
}
