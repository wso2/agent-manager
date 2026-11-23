import { NewProjectPage, EditProjectPage, ProjectOverviewPage } from '../pages/ProjectFormPages';
import { ProjectsPage } from '../pages/ProjectsPage';
import { uniqueName } from '../support/env';
import { expect, test } from '../support/fixtures';

test.describe('projects', () => {
  const id = uniqueName('p');
  const displayName = `UI e2e ${id}`;

  test.afterAll(async ({ api }) => {
    if ((await api.getProject(id)).status === 200) await api.deleteProject(id);
  });

  test('create, edit and delete a project', async ({ page, api }) => {
    await test.step('create', async () => {
      const form = new NewProjectPage(page);
      await form.goto();
      await expect(form.heading).toBeVisible();
      await form.createProject(displayName, id, 'Created by the UI e2e suite');
      await expect(page).toHaveURL(new RegExp(`/project/${id}(/|$)`));
      const got = await api.getProject(id);
      expect(got.status).toBe(200);
      expect(got.body.displayName).toBe(displayName);
    });

    await test.step('listed', async () => {
      const list = new ProjectsPage(page);
      await list.goto();
      await list.find(displayName);
      await expect(list.card(displayName)).toBeVisible();
    });

    const renamed = `${displayName} renamed`;
    await test.step('edit', async () => {
      const overview = new ProjectOverviewPage(page);
      await overview.goto(id);
      await expect(overview.title(displayName)).toBeVisible();
      await overview.edit.click();
      const edit = new EditProjectPage(page);
      await expect(edit.heading).toBeVisible();
      await edit.name.fill(renamed);
      await edit.description.fill('Edited by the UI e2e suite');
      await edit.update.click();
      await expect(overview.title(renamed)).toBeVisible();
      const got = await api.getProject(id);
      expect(got.body.displayName).toBe(renamed);
      expect(got.body.description).toBe('Edited by the UI e2e suite');
    });

    await test.step('delete', async () => {
      const list = new ProjectsPage(page);
      await list.goto();
      await list.delete(renamed);
      await expect(list.card(renamed)).toHaveCount(0);
      await expect.poll(async () => (await api.getProject(id)).status).toBe(404);
    });
  });
});
