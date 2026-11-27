import { type Locator, type Page } from '@playwright/test';
import { orgName } from '../support/env';

export class GatewaysPage {
  readonly heading: Locator;
  readonly add: Locator;
  readonly search: Locator;

  constructor(readonly page: Page) {
    this.heading = page.getByRole('heading', { name: 'Gateways', exact: true });
    this.add = page.getByRole('button', { name: 'Add gateway' });
    this.search = page.getByRole('textbox', { name: 'Search gateways' });
  }

  async goto(): Promise<void> {
    await this.page.goto(`/org/${orgName}/gateways`);
  }

  row(displayName: string): Locator {
    return this.page.getByRole('row').filter({ hasText: displayName });
  }

  async remove(displayName: string): Promise<void> {
    const row = this.row(displayName);
    // The row's actions show on hover.
    await row.hover();
    await row.getByRole('button', { name: 'Delete' }).click();
    await this.page.getByRole('dialog', { name: 'Delete Gateway' })
      .getByRole('button', { name: 'Delete' }).click();
  }
}

/** The Add gateway page; creating a gateway opens its setup page. */
export class AddGatewayPage {
  readonly displayName: Locator;
  readonly name: Locator;
  readonly publicUrl: Locator;
  readonly description: Locator;
  readonly create: Locator;

  constructor(readonly page: Page) {
    this.displayName = page.getByRole('textbox', { name: 'Display Name' });
    this.name = page.getByRole('textbox', { name: 'Name', exact: true });
    this.publicUrl = page.getByRole('textbox', { name: 'Public URL' });
    this.description = page.getByRole('textbox', { name: 'Description' });
    this.create = page.getByRole('button', { name: 'Create gateway', exact: true });
  }
}

/** The setup page a new self-hosted gateway opens on, with its one-time token. */
export class SetupGatewayPage {
  readonly created: Locator;
  readonly copyConfigure: Locator;
  readonly finishLater: Locator;

  constructor(readonly page: Page) {
    this.created = page.getByText(/Gateway created\./);
    this.copyConfigure = page.getByRole('button', { name: 'Copy configure command' });
    // Done stays disabled until the gateway connects, which it never does here.
    this.finishLater = page.getByRole('button', { name: 'Finish later' });
  }
}

export class GatewayViewPage {
  readonly newToken: Locator;
  readonly tokenCreated: Locator;

  constructor(readonly page: Page) {
    this.newToken = page.getByRole('button', { name: 'New token' });
    this.tokenCreated = page.getByText(/Token created\. Copy it now/);
  }

  revoke(): Locator {
    return this.page.getByRole('button', { name: 'Revoke token' });
  }
}
