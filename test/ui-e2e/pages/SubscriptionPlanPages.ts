import { type Locator, type Page } from '@playwright/test';
import { orgName } from '../support/env';

// Organization › Manage › Subscription Plans (the AI Hub module).
export class SubscriptionPlansPage {
  readonly heading: Locator;
  readonly search: Locator;
  readonly create: Locator;

  constructor(readonly page: Page) {
    this.heading = page.getByRole('heading', { name: 'Subscription Plans', exact: true });
    this.search = page.getByRole('textbox', { name: 'Search plans' });
    this.create = page.getByRole('link', { name: 'Create plan' });
  }

  async goto(): Promise<void> {
    await this.page.goto(`/org/${orgName}/subscription-plans`);
  }

  /** Narrows the list to one plan, so its row is on screen whatever else exists. */
  async find(displayName: string): Promise<void> {
    await this.search.fill(displayName);
  }

  row(displayName: string): Locator {
    return this.page.getByRole('row').filter({ hasText: displayName });
  }

  /** The row's status switch, named "<plan>, active" or "<plan>, inactive". */
  status(displayName: string): Locator {
    return this.page.getByLabel(new RegExp(`^${displayName}, (active|inactive)$`));
  }

  edit(displayName: string): Locator {
    return this.page.getByRole('button', { name: `Edit ${displayName}` });
  }

  deprecateButton(displayName: string): Locator {
    return this.page.getByRole('button', { name: `Deprecate ${displayName}` });
  }

  /** Deprecates the plan through its confirmation, which wants the name typed. */
  async deprecate(displayName: string): Promise<void> {
    await this.deprecateButton(displayName).click();
    const dialog = this.page.getByRole('dialog', { name: 'Deprecate plan' });
    await dialog.getByRole('textbox', { name: 'Type the plan name to confirm' }).fill(displayName);
    await dialog.getByRole('button', { name: 'Deprecate' }).click();
  }
}

// The add and edit pages of a plan.
export class PlanFormPage {
  readonly name: Locator;
  readonly description: Locator;
  readonly conditionTypes: Locator;
  readonly count: Locator;
  readonly add: Locator;
  readonly save: Locator;

  constructor(readonly page: Page) {
    this.name = page.getByRole('textbox', { name: 'Plan name' });
    this.description = page.getByRole('textbox', { name: 'Description (optional)' });
    this.conditionTypes = page.getByRole('group', { name: 'Condition types' });
    this.count = page.getByRole('spinbutton', { name: 'Count' });
    this.add = page.getByRole('button', { name: 'Add plan' });
    this.save = page.getByRole('button', { name: 'Save changes' });
  }

  conditionType(name: string): Locator {
    return this.conditionTypes.getByRole('button', { name: new RegExp(`^${name}`) });
  }
}

// An MCP server's Subscription Plans tab.
export class McpServerPlansTab {
  readonly save: Locator;

  constructor(readonly page: Page) {
    this.save = page.getByRole('button', { name: 'Save', exact: true });
  }

  /** Closes the dialog that offers a picked plan's policies, when it opens. */
  async skipPolicies(): Promise<void> {
    const dialog = this.page.getByRole('dialog');
    const opened = await dialog.waitFor({ state: 'visible', timeout: 3_000 }).then(() => true, () => false);
    if (opened) await dialog.getByRole('button', { name: 'Skip' }).click();
  }

  async goto(proxyHandle: string): Promise<void> {
    await this.page.goto(`/org/${orgName}/mcp-proxies/view/${proxyHandle}?tab=subscription-plans`);
  }

  /** A plan's card, a checkbox named after the plan. */
  card(displayName: string): Locator {
    return this.page.getByRole('checkbox', { name: displayName, exact: true });
  }
}
