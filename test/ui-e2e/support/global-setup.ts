import { request, type FullConfig } from '@playwright/test';
import { mkdirSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { adminUser, baseURL, resolveAdminPassword } from './env';

export const STORAGE_STATE = path.join(__dirname, '..', '.auth', 'admin.json');

// Signs the admin in once through the API and stores the session the way the
// console's file sign-in does (localStorage `amp.file-auth.session`), so every
// spec but the sign-in journey starts signed in.
export default async function globalSetup(_config: FullConfig): Promise<void> {
  // Workers inherit the environment set here.
  process.env.AMP_ADMIN_PASSWORD = resolveAdminPassword();

  const ctx = await request.newContext({ baseURL });
  const health = await ctx.get('/healthz');
  if (!health.ok()) {
    throw new Error(`the stack at ${baseURL} is not up (healthz ${health.status()}); run make up`);
  }
  const res = await ctx.post('/auth/login', {
    data: { username: adminUser, password: process.env.AMP_ADMIN_PASSWORD },
  });
  if (res.status() !== 200) {
    throw new Error(`admin sign-in failed (${res.status()}); the stack must run with file sign-in`);
  }
  const { access_token: accessToken, expires_in: expiresIn } = await res.json();
  await ctx.dispose();

  const session = { accessToken, expiresAt: Date.now() + (expiresIn ?? 3600) * 1000 };
  mkdirSync(path.dirname(STORAGE_STATE), { recursive: true });
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
