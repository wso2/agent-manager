import { type Locator, type Page } from '@playwright/test';
import { orgName } from '../support/env';

export class OrgOverviewPage {
  readonly greeting: Locator;
  readonly projectsHeading: Locator;
  readonly createProject: Locator;

  constructor(readonly page: Page) {
    this.greeting = page.getByRole('heading', { level: 1, name: /^Hello/ });
    this.projectsHeading = page.getByRole('heading', { name: 'Projects', exact: true });
    this.createProject = page.getByRole('link', { name: 'Create project' })
      .or(page.getByRole('button', { name: 'Create project' }));
  }

  async goto(): Promise<void> {
    await this.page.goto(`/org/${orgName}`);
  }
}
