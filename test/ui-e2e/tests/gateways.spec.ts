import { AddGatewayDrawer, GatewaysPage, GatewayViewPage } from '../pages/GatewayPages';
import { uniqueName } from '../support/env';
import { expect, test } from '../support/fixtures';

test.use({ permissions: ['clipboard-read', 'clipboard-write'] });

test.describe('gateways', () => {
  const name = uniqueName('gw');
  const displayName = `UI e2e ${name}`;

  test.afterAll(async ({ api }) => {
    await api.delete(`${api.gw}/gateways/${name}`);
  });

  test('register a self-hosted gateway, issue a token and remove it', async ({ page, api }) => {
    const gateways = new GatewaysPage(page);
    const drawer = new AddGatewayDrawer(page);

    await test.step('register', async () => {
      await gateways.goto();
      await expect(gateways.heading).toBeVisible();
      await gateways.add.click();
      await drawer.displayName.fill(displayName);
      await drawer.name.fill(name);
      await drawer.publicUrl.fill('https://uie2e-gateway.example.com');
      await drawer.description.fill('Registered by the UI e2e suite');
      await drawer.create.click();
      await expect(drawer.created).toBeVisible();

      // The configure command carries the registration token.
      await drawer.copyConfigure.click();
      const command = await page.evaluate(() => navigator.clipboard.readText());
      expect(command.length).toBeGreaterThan(20);
      await drawer.done.click();
      await expect(gateways.row(displayName)).toBeVisible();
    });

    await test.step('issue and revoke a registration token', async () => {
      await gateways.row(displayName).getByText(displayName).click();
      const view = new GatewayViewPage(page);
      await view.newToken.click();
      await expect(view.tokenCreated).toBeVisible();
      const revoke = view.revoke();
      const before = await revoke.count();
      await revoke.first().click();
      await page.getByRole('dialog', { name: 'Revoke token' })
        .getByRole('button', { name: 'Revoke' }).click();
      await expect(page.getByText('Revoked').first()).toBeVisible();
      await expect(revoke).toHaveCount(before - 1);
    });

    await test.step('remove', async () => {
      await gateways.goto();
      await gateways.remove(displayName);
      await expect(gateways.row(displayName)).toHaveCount(0);
      await expect.poll(async () => (await api.get(`${api.gw}/gateways/${name}`)).status).toBe(404);
    });
  });
});
