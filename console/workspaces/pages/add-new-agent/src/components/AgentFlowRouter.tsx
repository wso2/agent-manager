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

