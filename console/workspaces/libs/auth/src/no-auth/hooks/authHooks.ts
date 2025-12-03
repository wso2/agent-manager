/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */


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
