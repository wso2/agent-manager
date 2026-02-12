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

import { useState, useCallback } from 'react';

export function useDirtyState<T extends Record<string, any>>(initialData: T) {
  const [isDirty, setIsDirty] = useState(false);
  const [initialState] = useState(JSON.stringify(initialData));

  const checkDirty = useCallback((currentData: T) => {
    const currentState = JSON.stringify(currentData);
    setIsDirty(currentState !== initialState);
  }, [initialState]);

  const resetDirty = useCallback(() => {
    setIsDirty(false);
  }, []);

  return {
    isDirty,
    checkDirty,
    resetDirty,
  };
}
