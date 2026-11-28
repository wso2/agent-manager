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
  /** A listed token: only its start, then a mask. */
  readonly tokenPrefixes: Locator;
  readonly addIdentityProvider: Locator;

  constructor(readonly page: Page) {
    this.newToken = page.getByRole('button', { name: 'New token' });
    this.tokenCreated = page.getByText(/Token created\. Copy it now/);
    this.tokenPrefixes = page.getByTitle('Only the start of the token is shown');
    this.addIdentityProvider = page.getByRole('button', { name: 'Add identity provider' });
  }

  revoke(): Locator {
    return this.page.getByRole('button', { name: 'Revoke token' });
  }

  identityProviderRow(name: string): Locator {
    return this.page.getByRole('row').filter({ hasText: name });
  }

  async removeIdentityProvider(name: string): Promise<void> {
    // The row's remove action shows on hover.
    const row = this.identityProviderRow(name);
    await row.hover();
    await row.getByRole('button', { name: 'Remove identity provider' }).click();
  }
}

/** The drawer that adds or removes a gateway's identity provider. */
export class IdentityProviderDrawer {
  readonly name: Locator;
  readonly issuer: Locator;
  readonly jwksUri: Locator;
  /** A self-hosted gateway's owner applies the change to its config.toml. */
  readonly addManualNote: Locator;
  readonly removeManualNote: Locator;
  readonly add: Locator;
  readonly remove: Locator;

  constructor(readonly page: Page) {
    this.name = page.getByPlaceholder('e.g., AcmeIdP');
    this.issuer = page.getByPlaceholder('https://idp.example.com', { exact: true });
    this.jwksUri = page.getByPlaceholder('https://idp.example.com/oauth2/jwks');
    this.addManualNote = page.getByText("Add this entry to the gateway's configs/config.toml");
    this.removeManualNote = page.getByText("Remove this entry from the gateway's configs/config.toml");
    this.add = page.getByRole('button', { name: 'Add', exact: true });
    this.remove = page.getByRole('button', { name: 'Remove', exact: true });
  }

  /** The config.toml entry the drawer shows for the gateway's owner. */
  snippet(text: string): Locator {
    return this.page.getByText(text).last();
  }
}
