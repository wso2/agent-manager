import { AuthProvider as AsgardeoAuthProvider } from '@asgardeo/auth-react';
import { globalConfig } from '@agent-management-platform/types';
import { AuthProviderProps } from '../types';

export const AuthProvider = ({ children }: AuthProviderProps) => {
  const { authConfig } = globalConfig;

  return (
    <AsgardeoAuthProvider config={authConfig}>
      {children}
    </AsgardeoAuthProvider>
  );
};

