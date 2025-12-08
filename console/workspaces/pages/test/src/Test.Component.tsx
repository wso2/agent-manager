import React from 'react';
import { AgentTest } from './AgentTest/AgentTest';
import { FadeIn } from '@agent-management-platform/views';

export const TestComponent: React.FC = () => {
  return (
    <FadeIn>
        <AgentTest />
    </FadeIn>
  );
};

export default TestComponent;
