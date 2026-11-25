import { type Locator, type Page } from '@playwright/test';
import { orgName } from '../support/env';

export class AddLlmProviderPage {
  readonly heading: Locator;
  readonly name: Locator;
  readonly apiKey: Locator;
  readonly add: Locator;

  constructor(readonly page: Page) {
    this.heading = page.getByRole('heading', { name: 'Add LLM Service Provider' });
    this.name = page.getByLabel(/^Name/);
    this.apiKey = page.getByLabel(/^API key/);
    this.add = page.getByRole('button', { name: 'Add provider' });
  }

  async goto(): Promise<void> {
    await this.page.goto(`/org/${orgName}/llm-providers/add`);
  }

  template(name: string): Locator {
    return this.page.getByRole('button', { name: new RegExp(`^${name}\\b`) }).first();
  }
}

export class ConfigureAgentPage {
  readonly heading: Locator;
  readonly addConfig: Locator;

  constructor(readonly page: Page) {
    this.heading = page.getByRole('heading', { name: 'Configure Agent' });
    this.addConfig = page.getByRole('button', { name: 'Add LLM configuration' })
      .or(page.getByRole('link', { name: 'Add LLM configuration' })).first();
  }

  async goto(project: string, agent: string): Promise<void> {
    await this.page.goto(`/org/${orgName}/project/${project}/agents/${agent}/configure`);
  }

  config(name: string): Locator {
    return this.page.getByText(name, { exact: true });
  }
}

export class AddLlmConfigPage {
  readonly heading: Locator;
  readonly selectProvider: Locator;
  readonly providerSearch: Locator;
  readonly name: Locator;
  readonly save: Locator;

  constructor(readonly page: Page) {
    this.heading = page.getByRole('heading', { name: 'Add LLM Configuration' });
    this.selectProvider = page.getByRole('button', { name: 'Select a Service Provider' });
    this.providerSearch = page.getByRole('textbox', { name: 'Search providers' });
    this.name = page.getByLabel(/^Name/);
    this.save = page.getByRole('button', { name: 'Save', exact: true });
  }

  async chooseProvider(displayName: string): Promise<void> {
    await this.selectProvider.click();
    await this.providerSearch.fill(displayName);
    await this.page.getByRole('button', { name: `${displayName}. Click to select` }).click();
  }

  selected(displayName: string): Locator {
    return this.page.getByRole('button', { name: `Selected: ${displayName}. Click to change.` });
  }
}
