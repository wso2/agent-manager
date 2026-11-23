import { type Locator, type Page } from '@playwright/test';
import { orgName } from '../support/env';

export class NewProjectPage {
  readonly heading: Locator;
  readonly name: Locator;
  readonly projectId: Locator;
  readonly editProjectId: Locator;
  readonly description: Locator;
  readonly create: Locator;

  constructor(readonly page: Page) {
    this.heading = page.getByRole('heading', { name: 'Create a Project' });
    this.name = page.getByLabel(/^Name/);
    this.projectId = page.getByLabel(/^Project ID/);
    this.editProjectId = page.getByRole('button', { name: 'Edit project ID' });
    this.description = page.getByLabel(/^Description/);
    this.create = page.getByRole('button', { name: 'Create', exact: true });
  }

  async goto(): Promise<void> {
    await this.page.goto(`/org/${orgName}/newProject`);
  }

  async createProject(displayName: string, id: string, description: string): Promise<void> {
    await this.name.fill(displayName);
    await this.editProjectId.click();
    await this.projectId.fill(id);
    await this.description.fill(description);
    await this.create.click();
  }
}

export class ProjectOverviewPage {
  readonly edit: Locator;

  constructor(readonly page: Page) {
    this.edit = page.getByRole('button', { name: 'Edit project' })
      .or(page.getByRole('link', { name: 'Edit project' }));
  }

  async goto(project: string): Promise<void> {
    await this.page.goto(`/org/${orgName}/project/${project}`);
  }

  title(displayName: string): Locator {
    return this.page.getByRole('heading', { level: 1, name: displayName });
  }
}

export class EditProjectPage {
  readonly heading: Locator;
  readonly name: Locator;
  readonly description: Locator;
  readonly update: Locator;

  constructor(readonly page: Page) {
    this.heading = page.getByRole('heading', { name: 'Edit Project' });
    this.name = page.getByLabel(/^Name/);
    this.description = page.getByLabel(/^Description/);
    this.update = page.getByRole('button', { name: 'Update project' });
  }
}
