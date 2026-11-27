import { AddGatewayPage, GatewaysPage, GatewayViewPage, SetupGatewayPage } from '../pages/GatewayPages';
import { uniqueName } from '../support/env';
import { expect, test } from '../support/fixtures';

test.use({ permissions: ['clipboard-read', 'clipboard-write'] });

test.describe('gateways', () => {
  const name = uniqueName('gw');
  const displayName = `UI e2e ${name}`;
  let gatewayId: string | undefined;

  test.afterAll(async ({ api }) => {
    if (gatewayId) await api.delete(api.gatewayPath(gatewayId));
  });

  test('register a self-hosted gateway, issue a token and remove it', async ({ page, api }) => {
    const gateways = new GatewaysPage(page);
    const addPage = new AddGatewayPage(page);
    const setup = new SetupGatewayPage(page);

    await test.step('register', async () => {
      await gateways.goto();
      await expect(gateways.heading).toBeVisible();
      await gateways.add.click();
      await addPage.displayName.fill(displayName);
      await addPage.name.fill(name);
      await addPage.publicUrl.fill('https://uie2e-gateway.example.com');
      await addPage.description.fill('Registered by the UI e2e suite');
      await addPage.create.click();
      await expect(page).toHaveURL(/\/gateways\/setup\/[^/]+$/);
      gatewayId = page.url().split('/').pop();
      await expect(setup.created).toBeVisible();

      // The configure command carries the registration token.
      await setup.copyConfigure.click();
      const command = await page.evaluate(() => navigator.clipboard.readText());
      expect(command.length).toBeGreaterThan(20);
      await setup.finishLater.click();
      await expect(page).toHaveURL(/\/gateways\/view\/[^/]+$/);
      await gateways.goto();
      await expect(gateways.row(displayName)).toBeVisible();
    });

    await test.step('issue and revoke a registration token', async () => {
      await gateways.row(displayName).getByText(displayName).click();
      await expect(page).toHaveURL(/\/gateways\/view\/[^/]+$/);
      gatewayId = page.url().split('/').pop();
      const view = new GatewayViewPage(page);
      const revoke = view.revoke();
      // Registration issued the first token.
      await expect(revoke).toHaveCount(1);
      await view.newToken.click();
      await expect(view.tokenCreated).toBeVisible();
      await expect(revoke).toHaveCount(2);
      await revoke.first().click();
      await page.getByRole('dialog', { name: 'Revoke token' })
        .getByRole('button', { name: 'Revoke' }).click();
      await expect(page.getByText('Revoked').first()).toBeVisible();
      await expect(revoke).toHaveCount(1);
    });

    await test.step('remove', async () => {
      await gateways.goto();
      await gateways.remove(displayName);
      await expect(gateways.row(displayName)).toHaveCount(0);
      await expect.poll(async () => (await api.get(api.gatewayPath(gatewayId!))).status).toBe(404);
    });
  });
});
