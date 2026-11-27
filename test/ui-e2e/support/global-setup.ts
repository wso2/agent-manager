import { chromium, request, type FullConfig } from '@playwright/test';
import { mkdirSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { ThunderSignInPage } from '../pages/ThunderSignInPage';
import { adminUser, authMode, baseURL, orgName, resolveAdminPassword } from './env';

export const STORAGE_STATE = path.join(__dirname, '..', '.auth', 'admin.json');

// Signs the admin in once, so every spec but the sign-in journeys starts
// signed in: with file sign-in through the API, with an identity provider
// through the console's redirect to ThunderID.
export default async function globalSetup(_config: FullConfig): Promise<void> {
  // Workers inherit the environment set here.
  process.env.AMP_ADMIN_PASSWORD = resolveAdminPassword();

  const ctx = await request.newContext({ baseURL });
  const health = await ctx.get('/healthz');
  await ctx.dispose();
  if (!health.ok()) {
    throw new Error(`the stack at ${baseURL} is not up (healthz ${health.status()}); run make up`);
  }
  mkdirSync(path.dirname(STORAGE_STATE), { recursive: true });
  if (authMode === 'idp') {
    await idpSignIn();
  } else {
    await fileSignIn();
  }
}

// Stores the session the way the console's file sign-in does (localStorage
// `amp.file-auth.session`).
async function fileSignIn(): Promise<void> {
  const ctx = await request.newContext({ baseURL });
  const res = await ctx.post('/auth/login', {
    data: { username: adminUser, password: process.env.AMP_ADMIN_PASSWORD },
  });
  if (res.status() !== 200) {
    throw new Error(`admin sign-in failed (${res.status()}); the stack must run with file sign-in, or set AMP_AUTH_MODE=idp`);
  }
  const { access_token: accessToken, expires_in: expiresIn } = await res.json();
  await ctx.dispose();

  const session = { accessToken, expiresAt: Date.now() + (expiresIn ?? 3600) * 1000 };
  writeFileSync(
    STORAGE_STATE,
    JSON.stringify({
      cookies: [],
      origins: [
        {
          origin: new URL(baseURL).origin,
          localStorage: [{ name: 'amp.file-auth.session', value: JSON.stringify(session) }],
        },
      ],
    }),
  );
}

// Signs in through ThunderID in a browser and keeps what the console stored
// (the ThunderID SDK's session in localStorage) as the storage state.
async function idpSignIn(): Promise<void> {
  const browser = await chromium.launch();
  try {
    const page = await browser.newPage({ baseURL });
    await page.goto('/login');
    const thunder = new ThunderSignInPage(page);
    try {
      await thunder.waitFor();
    } catch {
      throw new Error(`the console did not send sign-in to ThunderID (at ${page.url()}); the stack must run with auth.mode idp, or unset AMP_AUTH_MODE`);
    }
    await thunder.signInAs(adminUser, process.env.AMP_ADMIN_PASSWORD!);
    // Back in the console, signed in, on the organization overview.
    await page.waitForURL(new RegExp(`^${new URL(baseURL).origin}/org/${orgName}$`));
    await page.context().storageState({ path: STORAGE_STATE });
  } finally {
    await browser.close();
  }
}
