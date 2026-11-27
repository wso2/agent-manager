import { type Locator, type Page } from '@playwright/test';

// ThunderID's sign-in page (/gate/signin), where the console sends people in
// idp mode.
export class ThunderSignInPage {
  readonly username: Locator;
  readonly password: Locator;
  readonly signIn: Locator;
  readonly error: Locator;

  constructor(readonly page: Page) {
    this.username = page.getByRole('textbox', { name: 'Username' });
    this.password = page.getByRole('textbox', { name: 'Password' });
    this.signIn = page.getByRole('button', { name: 'Sign In', exact: true });
    this.error = page.getByRole('alert');
  }

  async waitFor(): Promise<void> {
    await this.page.waitForURL(/\/gate\/signin/);
    await this.username.waitFor();
  }

  async signInAs(username: string, password: string): Promise<void> {
    await this.username.fill(username);
    await this.password.fill(password);
    await this.signIn.click();
  }
}
