import { ReactNode } from "react";

interface AuthProviderProps {
    children: ReactNode;
}

export type { AuthProviderProps };

export type UserInfo = {
    allowedScopes?: string;
    displayName?: string;
    familyName?: string;
    givenName?: string;
    jti?: string;
    orgHandle?: string;
    orgId?: string;
    orgName?: string;
    sessionState?: string;
    sub?: string;
    username?: string;
}
