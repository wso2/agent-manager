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

import { useCallback, useEffect, useMemo } from 'react';
import { generatePath, matchPath, useLocation, useNavigate } from 'react-router-dom';
import { absoluteRouteMap } from '@agent-management-platform/types';
import { UseFormReturn } from 'react-hook-form';
import { AddAgentFormValues } from '../form/schema';

export type AgentOption = 'new' | 'existing';
export type AgentOptionState = AgentOption | null;

const NEW_AGENT_ROUTES = absoluteRouteMap.children.org.children.projects.children.newAgent;
const CREATE_PATTERN = NEW_AGENT_ROUTES.children.create.path;
const CONNECT_PATTERN = NEW_AGENT_ROUTES.children.connect.path;

const isMatch = (pattern: string, pathname: string) => (
  matchPath({ path: pattern, end: true }, pathname) !== null
);

export const useAgentFlow = (
  methods: UseFormReturn<AddAgentFormValues>,
  orgId?: string,
  projectId?: string,
) => {
  const navigate = useNavigate();
  const location = useLocation();

  const selectedOption = useMemo<AgentOptionState>(() => {
    if (isMatch(CREATE_PATTERN, location.pathname)) {
      return 'new';
    }
    if (isMatch(CONNECT_PATTERN, location.pathname)) {
      return 'existing';
    }
    return null;
  }, [location.pathname]);

  useEffect(() => {
    if (selectedOption) {
      methods.setValue('deploymentType', selectedOption);
    }
  }, [methods, selectedOption]);

  const handleSelect = useCallback((option: AgentOption) => {
    methods.setValue('deploymentType', option);

    const target = option === 'new'
      ? CREATE_PATTERN
      : CONNECT_PATTERN;

    navigate(generatePath(target, {
      orgId: orgId ?? '',
      projectId: projectId ?? 'default',
    }));
  }, [methods, navigate, orgId, projectId]);

  return {
    selectedOption,
    handleSelect,
  };
};

