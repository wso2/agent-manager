
import { UserInfo } from '../../types';

const demoUserInfo : UserInfo = {
  username: 'john.doe',
  displayName: 'John Doe',
  orgHandle: 'default',
  orgId: 'default',
  orgName: 'Default',
  sessionState: '',
  sub: 'default',
  allowedScopes: "openid email profile",
};

export const useAuthHooks = () => {
  return {
    isAuthenticated: true,
    userInfo: demoUserInfo,
    isLoadingUserInfo: false,
    isLoadingIsAuthenticated: false,
    login: () => Promise.resolve(),
    logout: () => Promise.resolve(),
    trySignInSilently: () => Promise.resolve(),
    getToken: () => Promise.resolve('eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJpc3MiOiJBZ2VudCBNYW5hZ2VtZW50IFBsYXRmb3JtIExvY2FsIiwiaWF0IjoxNzYxNzI3NDY5LCJleHAiOjE3OTMyNjM0NjksImF1ZCI6ImxvY2FsaG9zdCIsInN1YiI6IjhmMzA3MzUxLTI1YzUtNGZjNi04NWUwLWY1MWMyZDQ1OGYwNiJ9.etSp2_pwhdaWnFlK8IYWCptWV1MiZd32Ou6Ri6rBIvE'),
  };
};
