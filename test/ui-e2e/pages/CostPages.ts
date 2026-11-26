import { type Locator, type Page } from '@playwright/test';
import { orgName } from '../support/env';

const costRoot = `/org/${orgName}/cost-management`;

export class BudgetsPage {
  readonly heading: Locator;
  readonly search: Locator;
  readonly statusFilter: Locator;
  readonly createMenu: Locator;

  constructor(readonly page: Page) {
    this.heading = page.getByRole('heading', { name: 'Budgets' });
    this.search = page.getByRole('textbox', { name: 'Search by name or label' });
    this.statusFilter = page.getByLabel('Filter by status');
    this.createMenu = page.getByRole('button', { name: /^Create budget/ });
  }

  async goto(): Promise<void> {
    await this.page.goto(`${costRoot}/budgets`);
  }

  /** Narrows the list to one budget, so its row is on screen whatever else exists. */
  async find(text: string): Promise<void> {
    await this.search.fill(text);
  }

  row(displayName: string): Locator {
    return this.page.getByRole('row').filter({ hasText: displayName });
  }

  async startCreate(): Promise<void> {
    await this.createMenu.click();
    await this.page.getByRole('menuitem', { name: /Create manually/ }).click();
  }
}

export class BudgetWizardPage {
  readonly name: Locator;
  readonly description: Locator;
  readonly labels: Locator;
  readonly next: Locator;
  readonly previous: Locator;
  readonly create: Locator;
  readonly save: Locator;
  readonly amount: Locator;
  readonly blockAt: Locator;
  readonly addFilter: Locator;

  constructor(readonly page: Page) {
    this.name = page.getByLabel('Name', { exact: true });
    this.description = page.getByLabel('Description (optional)');
    this.labels = page.getByRole('combobox', { name: 'Labels' });
    this.next = page.getByRole('button', { name: /^Next/ });
    this.previous = page.getByRole('button', { name: /Previous/ });
    this.create = page.getByRole('button', { name: 'Create budget' });
    this.save = page.getByRole('button', { name: /Save changes/ });
    this.amount = page.getByLabel('Amount');
    this.blockAt = page.getByLabel('Block at');
    this.addFilter = page.getByRole('button', { name: 'Add filter' });
  }

  step(text: string): Locator {
    return this.page.getByText(text);
  }

  async addLabel(label: string): Promise<void> {
    await this.labels.fill(label);
    await this.labels.press('Enter');
  }

  period(name: 'Daily' | 'Weekly' | 'Monthly'): Locator {
    return this.page.getByRole('button', { name });
  }

  enforcement(name: 'Block requests' | 'Allow requests'): Locator {
    return this.page.getByRole('radio', { name });
  }

  /** The select of a filter's type: the label sits on its field, the menu opens from the combobox inside. */
  private filterTypeSelect(position: number): Locator {
    return this.page
      .getByLabel(`Filter ${position} type`)
      .first()
      .locator('xpath=ancestor-or-self::*[contains(@class,"MuiFormControl-root")][1]')
      .getByRole('combobox');
  }

  /** Adds a filter of the given type (filters are numbered from 1). */
  async addFilterOfType(position: number, type: string): Promise<void> {
    await this.addFilter.click();
    await this.filterTypeSelect(position).click();
    await this.page.getByRole('option', { name: type }).click();
  }

  /** Types CEL expressions into a filter's chip field, each added with Enter. */
  async addExpressions(position: number, ...expressions: string[]): Promise<void> {
    // The filter's row holds its type select and its chip field.
    const row = this.filterTypeSelect(position).locator(
      'xpath=ancestor::*[contains(@class,"MuiStack-root")][1]',
    );
    const field = row.locator('input[role="combobox"]');
    for (const expression of expressions) {
      await field.fill(expression);
      await field.press('Enter');
    }
  }
}

export class BudgetDetailPage {
  readonly edit: Locator;
  readonly disable: Locator;
  readonly enable: Locator;
  readonly remove: Locator;
  readonly inactiveNotice: Locator;

  constructor(readonly page: Page) {
    this.edit = page.getByRole('button', { name: 'Edit' });
    this.disable = page.getByRole('button', { name: 'Disable' });
    this.enable = page.getByRole('button', { name: 'Enable' });
    this.remove = page.getByRole('button', { name: 'Delete' });
    this.inactiveNotice = page.getByText(/Disabled: not counting or blocking/);
  }

  async goto(name: string): Promise<void> {
    await this.page.goto(`${costRoot}/budgets/view/${name}`);
  }

  async confirmDelete(): Promise<void> {
    const dialog = this.page.getByRole('dialog', { name: 'Delete Budget?' });
    await dialog.getByRole('button', { name: 'Delete' }).click();
  }
}
