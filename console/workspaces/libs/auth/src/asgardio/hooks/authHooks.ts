import { useAuthContext } from '@asgardeo/auth-react';
import { useQuery } from '@tanstack/react-query';
import { UserInfo } from '../../types';

export const useAuthHooks = () => {
  const { 
      signIn, 
      signOut,
      getAccessToken,
      getBasicUserInfo, 
      isAuthenticated,
      trySignInSilently,
    } = useAuthContext() ?? {};

  const { data: userInfo , isLoading: isLoadingUserInfo } = useQuery({
    queryKey: ['auth', 'userInfo'],
    queryFn: () => {
      return getBasicUserInfo();
    },
  });

  const { 
      data: isAuthenticatedState,
      isLoading: isLoadingIsAuthenticated,
      refetch: refetchIsAuthenticated 
    } = useQuery({
    queryKey: ['isAuthenticated',isAuthenticated],
    queryFn: () => {
      return isAuthenticated();
    },
  });

  const customLogin = () => {
    signIn();
    refetchIsAuthenticated();
  };
  return {
    isAuthenticated: isAuthenticatedState,
    userInfo: userInfo as UserInfo,
    isLoadingUserInfo: isLoadingUserInfo,
    isLoadingIsAuthenticated: isLoadingIsAuthenticated,
    getToken: () => getAccessToken(),
    login: () => customLogin(),
    logout: () => signOut(),
    trySignInSilently: () => trySignInSilently(),
  };
};
