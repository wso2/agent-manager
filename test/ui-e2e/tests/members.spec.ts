import { AddUserPage, InviteUserPage, RolePage, UsersPage } from '../pages/IdentityPages';
import { uniqueName } from '../support/env';
import { expect, test } from '../support/fixtures';

test.describe('members', () => {
  const username = uniqueName('user').replace(/-/g, '.');

  test.afterAll(async ({ api }) => {
    await api.deleteUser(username);
  });

  test('invite a user by email', async ({ page }) => {
    const users = new UsersPage(page);
    await users.goto();
    await users.inviteUser.click();
    const invite = new InviteUserPage(page);
    const email = `${uniqueName('inv')}@example.com`;
    await invite.email.fill(email);
    await invite.getLink.click();
    await expect(page.getByText(`An invitation has been created for ${email}`)).toBeVisible();
    await expect(invite.link).toHaveValue(/^https?:\/\//);
  });

  test('add a user and assign a role; the user shows in the member list with it', async ({ page, api }) => {
    const users = new UsersPage(page);

    await users.goto();
    await users.addUser.click();
    const add = new AddUserPage(page);
    await add.username.fill(username);
    await add.password.fill(`Uie2e-${Date.now()}-Pw!`);
    await add.create.click();
    // Success returns to the member list.
    await expect(page).toHaveURL(/\/settings\/identities\/users$/);
    await users.search.fill(username);
    await expect(users.row(username)).toBeVisible();

    // Assign a role from the role's page.
    const roles = await api.get(`${api.org}/identities/roles`);
    expect(roles.status).toBe(200);
    const list: any[] = roles.body.roles ?? roles.body;
    const role = list.find((r) => r.name === 'Developer') ?? list.find((r) => !/admin/i.test(r.name));
    const rolePage = new RolePage(page);
    await rolePage.goto(role.id);
    await rolePage.usersTab.click();
    await rolePage.addUser.fill(username);
    await page.getByRole('option', { name: new RegExp(username) }).click();
    await expect(page.getByRole('row').filter({ hasText: username })).toBeVisible();
    await rolePage.save.click();
    await expect(rolePage.updated).toBeVisible();

    // The member list leads to the user, whose roles now include it.
    await users.goto();
    await users.search.fill(username);
    await users.row(username).getByText(username).click();
    await page.getByRole('tab', { name: 'Roles' }).click();
    await expect(page.getByText(role.displayName ?? role.name, { exact: true }).first()).toBeVisible();
  });
});
