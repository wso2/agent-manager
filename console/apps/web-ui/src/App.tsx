import { CoreUI as AgentManagerApp } from '@agent-management-platform/am-core-ui';
import { useScopeAccess } from './access/useScopeAccess';

function App() {
  return <AgentManagerApp useFeatureAccess={useScopeAccess} />
}

export default App
