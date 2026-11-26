import { AgentOverviewPage, RegisterAgentPage, SetupAgentDrawer } from '../pages/AgentPages';
import { uniqueName } from '../support/env';
import { expect, test } from '../support/fixtures';

test.use({ permissions: ['clipboard-read', 'clipboard-write'] });

test('register an external agent and copy its connection token', async ({ page, api, workerProject }) => {
  const handle = uniqueName('agent');
  const displayName = `Agent ${handle}`;

  const form = new RegisterAgentPage(page);
  await form.goto(workerProject);
  await expect(form.heading).toBeVisible();
  await form.register(displayName, handle);

  // Registration opens the agent with its setup drawer.
  await expect(page).toHaveURL(new RegExp(`/agents/${handle}\\?setup=true`));
  const drawer = new SetupAgentDrawer(page);
  await expect(drawer.root).toBeVisible();

  const got = await api.getAgent(workerProject, handle);
  expect(got.status).toBe(200);
  expect(got.body.spec.displayName).toBe(displayName);

  // The drawer generates a token on open; copy it.
  await expect(drawer.generated).toBeVisible();
  await drawer.copyToken.click();
  const token = await page.evaluate(() => navigator.clipboard.readText());
  expect(token.split('.'), 'the copied token is a JWT').toHaveLength(3);

  await drawer.close.click();
  await expect(drawer.root).toBeHidden();
  await expect(new AgentOverviewPage(page).setUp).toBeVisible();
});
