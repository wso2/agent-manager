import { type Locator, type Page } from '@playwright/test';
import { orgName } from '../support/env';

export class ProjectsPage {
  readonly heading: Locator;
  readonly search: Locator;

  constructor(readonly page: Page) {
    this.heading = page.getByRole('heading', { name: 'All Projects' });
    this.search = page.getByRole('textbox', { name: 'Search projects' });
  }

  async goto(): Promise<void> {
    await this.page.goto(`/org/${orgName}/projects`);
  }

  /** Narrows the list to one project, so the card is on screen whatever else exists. */
  async find(displayName: string): Promise<void> {
    await this.search.fill(displayName);
  }

  card(displayName: string): Locator {
    return this.page.getByText(displayName, { exact: true });
  }

  async delete(displayName: string): Promise<void> {
    await this.find(displayName);
    await this.page.getByRole('button', { name: 'Delete project' }).first().click();
    const dialog = this.page.getByRole('dialog', { name: 'Delete Project?' });
    await dialog.getByRole('button', { name: 'Delete' }).click();
  }
}
