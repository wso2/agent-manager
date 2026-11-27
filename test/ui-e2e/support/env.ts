import { execFileSync } from 'node:child_process';

// Where the stack serves the API and the console. AMP_BASE_URL wins; else
// AMP_API_PORT, the same setting deployments/standalone/.env uses.
export const baseURL =
  process.env.AMP_BASE_URL ?? `http://localhost:${process.env.AMP_API_PORT ?? '9000'}`;

export const orgName = process.env.AMP_ORG ?? 'default';

// How the stack signs people in: file (the standalone default) or idp. Users,
// groups and roles live in the identity provider, so their journeys need idp.
export const authMode = process.env.AMP_AUTH_MODE ?? 'file';
export const adminUser = process.env.AMP_ADMIN_USER ?? 'admin';

// The admin password: AMP_ADMIN_PASSWORD if set, else the one amp-init
// generated, read from the running amp-api container the way
// deployments/standalone/scripts/smoke.sh does.
export function resolveAdminPassword(): string {
  if (process.env.AMP_ADMIN_PASSWORD) return process.env.AMP_ADMIN_PASSWORD;
  const projects = [process.env.COMPOSE_PROJECT_NAME, 'amp-lite'].filter(Boolean) as string[];
  for (const project of projects) {
    try {
      const id = execFileSync('docker', [
        'ps', '-q',
        '--filter', `label=com.docker.compose.project=${project}`,
        '--filter', 'label=com.docker.compose.service=amp-api',
      ], { encoding: 'utf8' }).split('\n')[0].trim();
      if (!id) continue;
      const pw = execFileSync('docker', [
        'exec', id, 'cat', '/var/lib/agent-manager/secrets/admin-password',
      ], { encoding: 'utf8' }).trim();
      if (pw) return pw;
    } catch {
      // try the next project
    }
  }
  throw new Error('AMP_ADMIN_PASSWORD is not set and the generated one could not be read from amp-api');
}

export function adminPassword(): string {
  const pw = process.env.AMP_ADMIN_PASSWORD;
  if (!pw) throw new Error('AMP_ADMIN_PASSWORD is not set; global setup resolves it');
  return pw;
}

// ThunderID as the browser and the suite reach it, and the API's machine
// client, for idp mode. The defaults are the CI settings
// (.github/scripts/standalone-ci-settings.sh) and the Go e2e suite's.
export const thunderURL = process.env.AMP_THUNDER_URL ?? 'http://thunder.amp.localhost:8080';
export const idpClientId = process.env.AMP_IDP_CLIENT_ID ?? 'amp-api-client';
export const idpClientSecret = process.env.AMP_IDP_CLIENT_SECRET ?? 'amp-api-client-secret';

// Names every resource the suite creates, so leftovers are easy to find.
export const PREFIX = 'uie2e';
export function uniqueName(kind: string): string {
  const rand = Math.random().toString(36).slice(2, 7);
  return `${PREFIX}-${kind}-${Date.now().toString(36).slice(-4)}${rand}`;
}
