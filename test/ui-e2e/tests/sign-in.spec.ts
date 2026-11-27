import { expect, test } from '@playwright/test';
import { LoginPage } from '../pages/LoginPage';
import { OrgOverviewPage } from '../pages/OrgOverviewPage';
import { Shell } from '../pages/Shell';
import { ThunderSignInPage } from '../pages/ThunderSignInPage';
import { adminPassword, adminUser, authMode, orgName } from '../support/env';

// Signs in through the console's form, so it starts signed out (no stored
// session). The form is file sign-in's; with an identity provider the console
// sends people to it instead (below).
test.describe('sign-in', () => {
  test.skip(authMode !== 'file', 'the sign-in form is file sign-in (AMP_AUTH_MODE=file)');

  test('a wrong password is refused and the form stays', async ({ page }) => {
    const login = new LoginPage(page);
    await login.goto();
    await login.signInAs(adminUser, 'not-the-password');
    await expect(login.error).toBeVisible();
    await expect(page).toHaveURL(/\/login/);
    await expect(login.heading).toBeVisible();
  });

  test('a deep link while signed out goes to sign-in', async ({ page }) => {
    await page.goto(`/org/${orgName}/projects`);
    await expect(new LoginPage(page).heading).toBeVisible();
  });

  test('the admin signs in and reaches the organization overview', async ({ page }) => {
    const login = new LoginPage(page);
    await login.goto();
    await expect(login.signIn).toBeDisabled();
    await login.signInAs(adminUser, adminPassword());

    // Sign-in lands on the organization overview.
    const overview = new OrgOverviewPage(page);
    await expect(page).toHaveURL(new RegExp(`/org/${orgName}$`));
    await expect(overview.greeting).toBeVisible();
    await expect(overview.projectsHeading).toBeVisible();

    // The session survives a reload.
    await page.reload();
    await expect(overview.greeting).toBeVisible();
  });
});

// Signs in through ThunderID, the identity provider, starting signed out.
test.describe('sign-in through the identity provider', () => {
  test.skip(authMode !== 'idp', 'needs an identity provider (AMP_AUTH_MODE=idp)');

  test('a deep link while signed out goes to the identity provider and back', async ({ page }) => {
    await page.goto(`/org/${orgName}/projects`);
    const thunder = new ThunderSignInPage(page);
    await thunder.waitFor();
    await thunder.signInAs(adminUser, adminPassword());

    // Sign-in lands back in the console, signed in.
    const shell = new Shell(page);
    await expect(page).toHaveURL(new RegExp(`/org/${orgName}`));
    await expect(shell.org).toBeVisible();

    // The session survives a reload.
    await page.reload();
    await expect(shell.org).toBeVisible();
  });

  test('a wrong password is refused at the identity provider', async ({ page }) => {
    await page.goto('/login');
    const thunder = new ThunderSignInPage(page);
    await thunder.waitFor();
    await thunder.signInAs(adminUser, 'not-the-password');
    await expect(thunder.error).toBeVisible();
    await expect(page).toHaveURL(/\/gate\/signin/);
  });
});
