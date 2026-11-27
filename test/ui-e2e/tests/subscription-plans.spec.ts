import { McpServerPlansTab, PlanFormPage, SubscriptionPlansPage } from '../pages/SubscriptionPlanPages';
import { uniqueName } from '../support/env';
import { expect, test } from '../support/fixtures';

// Subscription plans belong to the AI Hub module, which a stack installs with
// `amp module install ai-hub`. A plan is never removed, only deprecated: it
// stays, read only, for the subscriptions on it, so these journeys leave their
// plans behind, deprecated.
test.describe.serial('Subscription plans', () => {
  const plan = uniqueName('plan');
  const retired = uniqueName('plan');
  const other = uniqueName('plan');
  const proxy = uniqueName('mcp');
  let proxyCreated = false;

  test.beforeAll(async ({ api }) => {
    const { status } = await api.get(api.plans);
    test.skip(status !== 200, 'the AI Hub module is not installed');
  });

  test.afterAll(async ({ api }) => {
    if (proxyCreated) await api.delete(`/m/ai-gateway/mcp-proxies/${proxy}`);
    for (const handle of [plan, retired, other]) await api.delete(`${api.plans}/${handle}`);
  });

  test('create a plan with a tool-call limit', async ({ page, api }) => {
    const list = new SubscriptionPlansPage(page);
    await list.goto();
    await expect(list.heading).toBeVisible();
    // An organization with no plans shows the create button in its empty state instead.
    await list.create.first().click();

    const form = new PlanFormPage(page);
    await form.name.fill(plan);
    await form.conditionType('Tool-call count').click();
    await form.count.fill('5');
    await form.add.click();

    await expect(page).toHaveURL(/\/subscription-plans$/);
    await list.find(plan);
    await expect(list.row(plan)).toBeVisible();
    await expect(list.status(plan)).toBeChecked();

    const { status, body } = await api.get(`${api.plans}/${plan}`);
    expect(status).toBe(200);
    expect(body.metadata.origin).toBe('ai-control-plane');
    expect(body.spec.conditions).toEqual([
      expect.objectContaining({ type: 'tool-call-count' }),
    ]);
    expect(body.status.deprecated).toBe(false);
  });

  test('edit the description, and deactivate and activate the plan', async ({ page, api }) => {
    const list = new SubscriptionPlansPage(page);
    await list.goto();
    await list.find(plan);
    await list.edit(plan).click();

    const form = new PlanFormPage(page);
    await expect(form.name).toHaveValue(plan);
    await form.description.fill('For the UI journey');
    await form.save.click();
    await expect(page).toHaveURL(/\/subscription-plans$/);
    await expect
      .poll(async () => (await api.get(`${api.plans}/${plan}`)).body?.spec?.description)
      .toBe('For the UI journey');

    await list.find(plan);
    await list.status(plan).click();
    await expect
      .poll(async () => (await api.get(`${api.plans}/${plan}`)).body?.spec?.status)
      .toBe('INACTIVE');
    await list.status(plan).click();
    await expect
      .poll(async () => (await api.get(`${api.plans}/${plan}`)).body?.spec?.status)
      .toBe('ACTIVE');
  });

  test('an MCP server offers active plans, never a deprecated one', async ({ page, api }) => {
    const target = await api.activeGateway();
    test.skip(!target, 'no active gateway serves the default environment');
    await api.createPlan(retired, { displayName: retired });
    expect(await api.delete(`${api.plans}/${retired}`)).toBe(200);
    await api.createMcpProxy(proxy, target!, [plan]);
    proxyCreated = true;

    const tab = new McpServerPlansTab(page);
    await tab.goto(proxy);
    await expect(tab.card(plan)).toHaveAttribute('aria-checked', 'true');
    await expect(tab.card(retired)).toHaveCount(0);
  });

  test('deprecate the plan: it stays, read only, and its offer stays', async ({ page, api }) => {
    const list = new SubscriptionPlansPage(page);
    await list.goto();
    await list.find(plan);
    await list.deprecate(plan);

    const row = list.row(plan);
    await expect(row.getByText('Deprecated', { exact: true })).toBeVisible();
    await expect(list.edit(plan)).toHaveCount(0);
    await expect(list.deprecateButton(plan)).toHaveCount(0);
    await expect(list.status(plan)).toHaveCount(0);
    await expect
      .poll(async () => (await api.get(`${api.plans}/${plan}`)).body?.status?.deprecated)
      .toBe(true);

    test.skip(!proxyCreated, 'no MCP server was created to offer the plan');
    const tab = new McpServerPlansTab(page);
    await tab.goto(proxy);
    const card = tab.card(plan);
    await expect(card).toHaveAttribute('aria-checked', 'true');
    await expect(card.getByText('Deprecated', { exact: true })).toBeVisible();
  });

  test('another plan beside the deprecated one saves only once it is removed', async ({ page, api }) => {
    test.skip(!proxyCreated, 'no MCP server was created to offer the plan');
    await api.createPlan(other, { displayName: other });
    const tab = new McpServerPlansTab(page);
    await tab.goto(proxy);
    await tab.card(other).click();
    // Ticking a plan may offer its policies; this journey is about the plans only.
    await tab.skipPolicies();
    await expect(tab.save).toBeDisabled();
    await expect(page.getByText(`"${plan}" is deprecated. Remove it to save`)).toBeVisible();

    await tab.card(plan).click();
    await expect(tab.save).toBeEnabled();
    await tab.save.click();
    await expect
      .poll(async () => (await api.get(`/m/ai-gateway/mcp-proxies/${proxy}`)).body?.spec?.subscriptionPlans)
      .toEqual([other]);
  });
});
