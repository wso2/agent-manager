import { type Locator, type Page } from '@playwright/test';
import { orgName } from '../support/env';

const base = `/org/${orgName}/settings/identities`;

export class UsersPage {
  readonly addUser: Locator;
  readonly inviteUser: Locator;
  readonly search: Locator;

  constructor(readonly page: Page) {
    this.addUser = page.getByRole('button', { name: 'Add user' })
      .or(page.getByRole('link', { name: 'Add user' })).first();
    this.inviteUser = page.getByRole('button', { name: 'Invite user' })
      .or(page.getByRole('link', { name: 'Invite user' })).first();
    this.search = page.getByRole('textbox', { name: 'Search users' });
  }

  async goto(): Promise<void> {
    await this.page.goto(`${base}/users`);
  }

  row(username: string): Locator {
    return this.page.getByRole('row').filter({ hasText: username });
  }
}

export class InviteUserPage {
  readonly email: Locator;
  readonly getLink: Locator;
  readonly link: Locator;

  constructor(readonly page: Page) {
    this.email = page.getByLabel(/^Email Address/);
    this.getLink = page.getByRole('button', { name: 'Get invitation link' });
    this.link = page.getByRole('textbox', { name: 'Invite Link' });
  }
}

export class AddUserPage {
  readonly username: Locator;
  readonly password: Locator;
  readonly create: Locator;

  constructor(readonly page: Page) {
    this.username = page.getByLabel(/^Username/);
    this.password = page.getByLabel(/^Password/);
    this.create = page.getByRole('button', { name: 'Create user' });
  }
}

export class RolePage {
  readonly usersTab: Locator;
  readonly addUser: Locator;
  readonly updated: Locator;

  constructor(readonly page: Page) {
    this.usersTab = page.getByRole('tab', { name: 'Users' });
    this.addUser = page.getByRole('combobox', { name: /Add User/i });
    this.updated = page.getByText('Role updated successfully.');
  }

  async goto(roleId: string): Promise<void> {
    await this.page.goto(`${base}/roles/${roleId}`);
  }
}
