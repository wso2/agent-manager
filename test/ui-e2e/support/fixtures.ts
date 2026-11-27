import { test as base } from '@playwright/test';
import { AmpApi } from './api';
import { uniqueName } from './env';
import { STORAGE_STATE } from './global-setup';

type WorkerFixtures = {
  /** API client signed in as the admin, for set-up, clean-up and read-back. */
  api: AmpApi;
  /** A project this worker owns, created through the API and removed after. */
  workerProject: string;
};

export const test = base.extend<{}, WorkerFixtures>({
  storageState: STORAGE_STATE,

  api: [
    async ({}, use) => {
      const api = await AmpApi.signIn();
      await use(api);
      await api.dispose();
    },
    { scope: 'worker' },
  ],

  workerProject: [
    async ({ api }, use) => {
      const name = uniqueName('proj');
      await api.createProject(name, `UI e2e ${name}`);
      await use(name);
      await api.deleteProject(name);
    },
    { scope: 'worker' },
  ],
});

export { expect } from '@playwright/test';
