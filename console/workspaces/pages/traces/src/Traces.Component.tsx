import React from 'react';
import { AgentTraces } from './AgentTraces/AgentTraces';
import { FadeIn } from '@agent-management-platform/views';

export const TracesComponent: React.FC = () => {
  return (
    <FadeIn>
      <AgentTraces />
    </FadeIn>
  );
};

export default TracesComponent;
