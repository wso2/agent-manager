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

