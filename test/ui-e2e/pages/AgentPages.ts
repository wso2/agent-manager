import { type Locator, type Page } from '@playwright/test';
import { orgName } from '../support/env';

export class RegisterAgentPage {
  readonly heading: Locator;
  readonly name: Locator;
  readonly handle: Locator;
  readonly editHandle: Locator;
  readonly create: Locator;

  constructor(readonly page: Page) {
    this.heading = page.getByRole('heading', { name: 'Register an Externally-Hosted Agent' });
    this.name = page.getByLabel(/^Name/);
    this.handle = page.getByLabel(/^Agent handle/);
    this.editHandle = page.getByRole('button', { name: 'Edit agent handle' });
    this.create = page.getByRole('button', { name: 'Create', exact: true });
  }

  /** The agent-type picker redirects here while external is the only type. */
  async goto(project: string): Promise<void> {
    await this.page.goto(`/org/${orgName}/project/${project}/newAgent`);
  }

  async register(displayName: string, handle: string): Promise<void> {
    await this.name.fill(displayName);
    await this.editHandle.click();
    await this.handle.fill(handle);
    await this.create.click();
  }
}

export class AgentOverviewPage {
  readonly setUp: Locator;

  constructor(readonly page: Page) {
    this.setUp = page.getByRole('button', { name: 'Set up agent' });
  }

  async goto(project: string, agent: string): Promise<void> {
    await this.page.goto(`/org/${orgName}/project/${project}/agents/${agent}`);
  }
}

// The drawer the agent page opens after registration (?setup=true).
export class SetupAgentDrawer {
  readonly root: Locator;
  readonly generate: Locator;
  readonly copyToken: Locator;
  readonly generated: Locator;
  readonly close: Locator;

  constructor(readonly page: Page) {
    this.root = page.getByRole('dialog').filter({ hasText: 'Setup Agent' })
      .or(page.getByRole('presentation').filter({ hasText: 'Setup Agent' })).first();
    this.generate = this.root.getByRole('button', { name: /^(Generate|Regenerate)$/ });
    this.copyToken = this.root.getByRole('button', { name: 'Copy code' });
    this.generated = this.root.getByText(/Token generated successfully/);
    this.close = this.root.getByRole('button', { name: 'Close' });
  }
}
