import { type Locator, type Page } from '@playwright/test';

// The navigation every signed-in page shares.
export class Shell {
  constructor(readonly page: Page) {}

  nav(name: string): Locator {
    return this.page.getByRole('link', { name, exact: true });
  }
}
