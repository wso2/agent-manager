import { APIRequestContext, expect, request } from '@playwright/test';
import { adminPassword, adminUser, baseURL, orgName } from './env';

// A thin API client the specs use to set up their own resources and to read
// back what the UI did, through a second channel.
export class AmpApi {
  private constructor(readonly ctx: APIRequestContext, readonly token: string) {}

  static async signIn(): Promise<AmpApi> {
    const anon = await request.newContext({ baseURL });
    const res = await anon.post('/auth/login', {
      data: { username: adminUser, password: adminPassword() },
    });
    expect(res.status(), 'admin sign-in through the API').toBe(200);
    const { access_token: token } = await res.json();
    await anon.dispose();
    const ctx = await request.newContext({
      baseURL,
      extraHTTPHeaders: { Authorization: `Bearer ${token}` },
    });
    return new AmpApi(ctx, token);
  }

  get org(): string {
    return `/api/v1/orgs/${orgName}`;
  }

  get gw(): string {
    return `/m/ai-gateway/orgs/${orgName}`;
  }

  async createProject(name: string, displayName = name): Promise<void> {
    const res = await this.ctx.post(`${this.org}/projects`, { data: { name, displayName } });
    expect(res.status(), await res.text()).toBe(202);
  }

  async getProject(name: string): Promise<{ status: number; body: any }> {
    const res = await this.ctx.get(`${this.org}/projects/${name}`);
    return { status: res.status(), body: res.ok() ? await res.json() : undefined };
  }

  async deleteProject(name: string): Promise<void> {
    // Agents first: a project with agents may refuse to go.
    const agents = await this.ctx.get(`${this.org}/projects/${name}/agents`);
    if (agents.ok()) {
      const body = await agents.json();
      for (const a of body.agents ?? []) {
        await this.ctx.delete(`${this.org}/projects/${name}/agents/${a.name}`);
      }
    }
    // Agent deletion finishes in the background, and a project with agents
    // left is refused: delete again until the project is gone.
    await expect
      .poll(
        async () => {
          await this.ctx.delete(`${this.org}/projects/${name}`);
          return (await this.ctx.get(`${this.org}/projects/${name}`)).status();
        },
        { timeout: 60_000, intervals: [1_000, 2_000, 5_000] },
      )
      .toBe(404);
  }

  /** Removes a user found by username, if it exists. */
  async deleteUser(username: string): Promise<void> {
    const res = await this.ctx.get(`${this.org}/identities/users`, { params: { limit: 200 } });
    const users: any[] = res.ok() ? (await res.json()).users ?? [] : [];
    const user = users.find((u) => u.attributes?.username === username);
    if (user) await this.ctx.delete(`${this.org}/identities/users/${user.id}`);
  }

  async createExternalAgent(project: string, name: string, displayName = name): Promise<void> {
    const res = await this.ctx.post(`${this.org}/projects/${project}/agents`, {
      data: {
        name,
        displayName,
        provisioning: { type: 'external' },
        agentType: { type: 'external-agent-api', subType: 'custom-api' },
      },
    });
    expect(res.status(), await res.text()).toBe(202);
  }

  async getAgent(project: string, name: string): Promise<{ status: number; body: any }> {
    const res = await this.ctx.get(`${this.org}/projects/${project}/agents/${name}`);
    return { status: res.status(), body: res.ok() ? await res.json() : undefined };
  }

  /** A budget by name, as the Budget kind (404 when there is none). */
  async getBudget(name: string): Promise<{ status: number; body: any }> {
    return this.get(`${this.gw}/budgets/${name}`);
  }

  async deleteBudget(name: string): Promise<number> {
    return this.delete(`${this.gw}/budgets/${name}`);
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

  async dispose(): Promise<void> {
    await this.ctx.dispose();
  }
}
