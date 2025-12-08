import { GlobalProviders } from './Providers';
import { RootRouter } from './Route';

function App() {

  return (
    <GlobalProviders >
      <RootRouter />
    </GlobalProviders>
  )
}

export default App
