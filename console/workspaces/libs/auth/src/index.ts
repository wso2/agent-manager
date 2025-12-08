import { AuthProvider as NoAuthAuthProvider } from './no-auth/AuthProvider';
import { useAuthHooks as useNoAuthHooks } from './no-auth/hooks/authHooks';

import { AuthProvider as AsgardeoAuthProvider } from './asgardio/AuthProvider';
import { useAuthHooks as useAsgardeoAuthHooks } from './asgardio/hooks/authHooks';
import { globalConfig } from '@agent-management-platform/types';

export const AuthProvider = globalConfig.disableAuth ? NoAuthAuthProvider : AsgardeoAuthProvider;
export const useAuthHooks = globalConfig.disableAuth ? useNoAuthHooks : useAsgardeoAuthHooks;

