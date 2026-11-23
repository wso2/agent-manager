import { expect, test } from '@playwright/test';
import { LoginPage } from '../pages/LoginPage';
import { OrgOverviewPage } from '../pages/OrgOverviewPage';
import { Shell } from '../pages/Shell';
import { adminPassword, adminUser, orgName } from '../support/env';

// Signs in through the form, so it starts signed out (no stored session).
test.describe('sign-in', () => {
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

    // Sign-in opens the organization's default project (or the project list).
    await expect(page).toHaveURL(new RegExp(`/org/${orgName}/`));
    await new Shell(page).nav('Overview').click();

    const overview = new OrgOverviewPage(page);
    await expect(page).toHaveURL(new RegExp(`/org/${orgName}$`));
    await expect(overview.greeting).toBeVisible();
    await expect(overview.projectsHeading).toBeVisible();

    // The session survives a reload.
    await page.reload();
    await expect(overview.greeting).toBeVisible();
  });
});
