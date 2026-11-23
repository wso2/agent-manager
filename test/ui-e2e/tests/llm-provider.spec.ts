import { AddLlmConfigPage, AddLlmProviderPage, ConfigureAgentPage } from '../pages/LlmPages';
import { orgName, uniqueName } from '../support/env';
import { expect, test } from '../support/fixtures';

test.use({ permissions: ['clipboard-read', 'clipboard-write'] });

// One provider and one agent, shared in order: the second journey uses the
// configuration the first one creates.
test.describe.serial('LLM providers and proxies', () => {
  const providerName = `UI e2e OpenAI ${uniqueName('llm')}`;
  const agent = uniqueName('llmagent');
  const configName = `cfg-${agent}`.slice(0, 40);
  let providerId: string | undefined;
  let configUrl: string | undefined;

  test.afterAll(async ({ api }) => {
    if (providerId) await api.delete(`/m/ai-gateway/llm-providers/${providerId}`);
  });

  test('add an organization LLM provider and attach it to an agent', async ({ page, api, workerProject }) => {
    await api.createExternalAgent(workerProject, agent);

    await test.step('add the provider', async () => {
      const add = new AddLlmProviderPage(page);
      await add.goto();
      await expect(add.heading).toBeVisible();
      await add.name.fill(providerName);
      await add.template('OpenAI').click();
      // A dummy key: nothing here calls the model.
      await add.apiKey.fill('sk-uie2e-not-a-real-key');
      await page.getByRole('checkbox').first().check();
      await add.add.click();
      await expect(page).toHaveURL(/\/llm-providers\/view\/[^/]+$/);
      providerId = page.url().split('/').pop();
      await expect(page.getByText(providerName).first()).toBeVisible();
    });

    await test.step('attach it to the agent', async () => {
      const configure = new ConfigureAgentPage(page);
      await configure.goto(workerProject, agent);
      await expect(configure.heading).toBeVisible();
      await configure.addConfig.click();

      const add = new AddLlmConfigPage(page);
      await expect(add.heading).toBeVisible();
      await add.chooseProvider(providerName);
      await expect(add.selected(providerName)).toBeVisible();
      await add.name.fill(configName);
      await add.save.click();
      await expect(page).toHaveURL(/\/configure\/llm-providers\/view\/[^/]+$/);
      await expect(page.getByRole('heading', { name: 'LLM Configuration' })).toBeVisible();
      configUrl = new URL(page.url()).pathname;
    });

    await test.step('the configuration is listed for the agent', async () => {
      const configure = new ConfigureAgentPage(page);
      await configure.goto(workerProject, agent);
      await expect(configure.config(configName)).toBeVisible();
      const got = await api.get(
        `${api.gw}/projects/${workerProject}/agents/${agent}/configurations?environment=default`,
      );
      expect(got.status).toBe(200);
      expect(JSON.stringify(got.body)).toContain(configName);
    });
  });

  // The console has no page of its own for project LLM proxies: attaching a
  // provider to an agent creates the proxy, and this is where a user sees it.
  test("the agent's LLM proxy issues an API key and the provider lists the agent", async ({ page }) => {
    test.skip(!configUrl, 'needs the configuration the previous journey creates');

    await test.step('generate the proxy API key', async () => {
      await page.goto(configUrl!);
      await page.getByRole('button', { name: 'Generate', exact: true }).click();
      await expect(page.getByText('You will only see this key once. Copy it now.')).toBeVisible();
      await page.getByRole('button', { name: 'Copy API key' }).click();
      const key = await page.evaluate(() => navigator.clipboard.readText());
      expect(key.length).toBeGreaterThan(16);
      // After a reload the key is managed, not shown again.
      await page.reload();
      await expect(page.getByRole('button', { name: 'Regenerate' })).toBeVisible();
      await expect(page.getByText('No API key generated yet')).toHaveCount(0);
    });

    await test.step("the provider's consumers include the agent", async () => {
      await page.goto(`/org/${orgName}/llm-providers/view/${providerId}`);
      await page.getByRole('tab', { name: 'Consumers' }).click();
      await expect(page.getByRole('link', { name: new RegExp(agent) })
        .or(page.getByRole('button', { name: new RegExp(agent) }))
        .or(page.getByText(agent)).first()).toBeVisible();
    });
  });
});
