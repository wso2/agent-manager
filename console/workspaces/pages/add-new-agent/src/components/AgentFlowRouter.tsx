import React from 'react';
import { Route, Routes } from 'react-router-dom';
import { UseFormReturn } from 'react-hook-form';
import { NewAgentOptions } from './NewAgentOptions';
import { NewAgentFromSource } from './NewAgentFromSource';
import { ConnectNewAgent } from './ConnectNewAgent';
import { AddAgentFormValues } from '../form/schema';
import { AgentOption } from '../hooks/useAgentFlow';

type AgentFlowRouterProps = {
  methods: UseFormReturn<AddAgentFormValues>;
  onSelect: (option: AgentOption) => void;
};

export const AgentFlowRouter: React.FC<AgentFlowRouterProps> = ({ methods, onSelect }) => (
  <Routes>
    <Route index element={<NewAgentOptions onSelect={onSelect} />} />
    <Route path="create" element={<NewAgentFromSource methods={methods} />} />
    <Route path="connect" element={<ConnectNewAgent methods={methods} />} />
  </Routes>
);

