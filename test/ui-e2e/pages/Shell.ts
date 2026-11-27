import { type Locator, type Page } from '@playwright/test';
import { orgName } from '../support/env';

// The header and navigation every signed-in page shares.
export class Shell {
  readonly org: Locator;

  constructor(readonly page: Page) {
    this.org = page.getByRole('button', { name: orgName, exact: true });
  }

  /** A side-navigation item (rendered as a button inside its link). */
  nav(name: string): Locator {
    return this.page.getByRole('navigation').getByRole('button', { name, exact: true });
  }
}
