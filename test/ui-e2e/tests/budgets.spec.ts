import { BudgetDetailPage, BudgetsPage, BudgetWizardPage } from '../pages/CostPages';
import { orgName, uniqueName } from '../support/env';
import { expect, test } from '../support/fixtures';

const CEL_MINI = "request.Path.startsWith('/mini/')";
const CEL_CLAUDE = "request.Path.startsWith('/claude/')";

test.describe('budgets', () => {
  const id = uniqueName('b');
  const displayName = `UI e2e budget ${id}`;
  // The console names a budget after its display name; the address shows the name.
  let name = '';

  test.afterAll(async ({ api }) => {
    if (name && (await api.getBudget(name)).status === 200) await api.deleteBudget(name);
  });

  test('create, view, edit, disable, enable and delete a budget', async ({ page, api }) => {
    await test.step('the first page asks for a name', async () => {
      const budgets = new BudgetsPage(page);
      await budgets.goto();
      await expect(budgets.heading).toBeVisible();
      await budgets.startCreate();
      const wizard = new BudgetWizardPage(page);
      await expect(wizard.step('Step 1 of 2 · Details')).toBeVisible();
      await wizard.next.click();
      // Still on the first page: the name is missing.
      await expect(wizard.step('Step 1 of 2 · Details')).toBeVisible();
      await expect(wizard.name).toHaveAttribute('aria-invalid', 'true');
    });

    await test.step('create a monthly Block budget over two CEL expressions', async () => {
      const wizard = new BudgetWizardPage(page);
      await wizard.name.fill(displayName);
      await wizard.description.fill('Created by the UI e2e suite');
      await wizard.addLabel('uie2e');
      await wizard.next.click();
      await expect(wizard.step('Step 2 of 2 · Applies to and limit')).toBeVisible();

      // An amount is needed before the budget is created.
      await wizard.create.click();
      await expect(wizard.amount).toHaveAttribute('aria-invalid', 'true');

      await wizard.addFilterOfType(1, 'Custom attribute');
      await wizard.addExpressions(1, CEL_MINI, CEL_CLAUDE);
      await wizard.amount.fill('50');
      await expect(wizard.enforcement('Block requests')).toBeChecked();
      await wizard.create.click();

      await expect(page).toHaveURL(/\/cost-management\/budgets\/view\/[^/]+$/);
      name = new URL(page.url()).pathname.split('/').pop() ?? '';
      expect(name).not.toBe('');

      // Read back through the API: the two expressions are alternatives in one filter.
      const got = await api.getBudget(name);
      expect(got.status).toBe(200);
      expect(got.body.spec.displayName).toBe(displayName);
      expect(got.body.spec.description).toBe('Created by the UI e2e suite');
      expect(got.body.metadata.labels).toEqual(['uie2e']);
      expect(got.body.spec.limit).toMatchObject({ amount: '50.00', period: 'month', action: 'block', blockAt: 100 });
      expect(got.body.spec.filters).toEqual([
        { type: 'attribute', expression: `(${CEL_MINI}) || (${CEL_CLAUDE})` },
      ]);
    });

    await test.step('the detail page shows it', async () => {
      await expect(page.getByText(displayName).first()).toBeVisible();
      await expect(page.getByText('Active', { exact: true })).toBeVisible();
      await expect(page.getByText('Blocks at $50.00 (100%).')).toBeVisible();
      await expect(page.getByText(CEL_MINI)).toBeVisible();
      await expect(page.getByText(CEL_CLAUDE)).toBeVisible();
    });

    await test.step('it is in the list', async () => {
      const budgets = new BudgetsPage(page);
      await budgets.goto();
      await budgets.find(displayName);
      await expect(budgets.row(displayName)).toBeVisible();
      await expect(budgets.row(displayName)).toContainText('$50');
    });

    await test.step('edit the description', async () => {
      const detail = new BudgetDetailPage(page);
      await detail.goto(name);
      await detail.edit.click();
      const wizard = new BudgetWizardPage(page);
      await expect(wizard.description).toHaveValue('Created by the UI e2e suite');
      await wizard.description.fill('Edited by the UI e2e suite');
      await wizard.save.click();
      await expect(page).toHaveURL(new RegExp(`/budgets/view/${name}$`));
      await expect
        .poll(async () => (await api.getBudget(name)).body?.spec.description)
        .toBe('Edited by the UI e2e suite');
    });

    await test.step('disable and enable it', async () => {
      const detail = new BudgetDetailPage(page);
      await detail.goto(name);
      await detail.disable.click();
      await expect(detail.inactiveNotice).toBeVisible();
      await expect
        .poll(async () => (await api.getBudget(name)).body?.spec.status)
        .toBe('inactive');
      await detail.enable.click();
      await expect(detail.inactiveNotice).toHaveCount(0);
      await expect
        .poll(async () => (await api.getBudget(name)).body?.spec.status)
        .toBe('active');
    });

    await test.step('delete it', async () => {
      const detail = new BudgetDetailPage(page);
      await detail.remove.click();
      await detail.confirmDelete();
      await expect(page).toHaveURL(new RegExp(`/org/${orgName}/cost-management/budgets$`));
      await expect.poll(async () => (await api.getBudget(name)).status).toBe(404);
    });
  });

  test('an Allow budget has no block share', async ({ page, api }) => {
    const allowName = uniqueName('a');
    let handle = '';
    try {
      const budgets = new BudgetsPage(page);
      await budgets.goto();
      await budgets.startCreate();
      const wizard = new BudgetWizardPage(page);
      await wizard.name.fill(`UI e2e allow ${allowName}`);
      await wizard.next.click();
      await expect(wizard.blockAt).toBeVisible();
      await wizard.enforcement('Allow requests').check();
      await expect(wizard.blockAt).toHaveCount(0);
      await wizard.period('Daily').click();
      await wizard.amount.fill('3');
      await wizard.create.click();
      await expect(page).toHaveURL(/\/cost-management\/budgets\/view\/[^/]+$/);
      handle = new URL(page.url()).pathname.split('/').pop() ?? '';
      const got = await api.getBudget(handle);
      expect(got.body.spec.limit).toMatchObject({ amount: '3.00', period: 'day', action: 'allow' });
      expect(got.body.spec.limit.blockAt).toBeUndefined();
      await expect(page.getByText('Only warns.')).toBeVisible();
    } finally {
      if (handle) await api.deleteBudget(handle);
    }
  });
});
