import { useState } from 'react'
import { RequireAuth } from '@biotron/style'
import TopBar from './components/TopBar'
import { type ViewMode } from './components/ModeToggle'
import { EXO_PERMISSIONS, PRODUCT_NAME } from './config'

// Placeholder until the machine list is fetched from the backend.
const MACHINES = ['TestDummyExo']

function Telemetry() {
  const [machine, setMachine] = useState(MACHINES[0])
  const [mode, setMode] = useState<ViewMode>('live')

  return (
    <div className="min-h-screen">
      <TopBar
        machines={MACHINES}
        selectedMachine={machine}
        onSelectMachine={setMachine}
        mode={mode}
        onModeChange={setMode}
      />
      <main className="mx-auto max-w-7xl px-6 py-10">
        <p className="text-sm text-slate-500 dark:text-muted">
          {mode === 'live' ? 'Live' : 'Historical'} view —{' '}
          <span className="text-slate-900 dark:text-white">{machine}</span>
        </p>
      </main>
    </div>
  )
}

function App() {
  return (
    <RequireAuth productName={PRODUCT_NAME} permission={EXO_PERMISSIONS} guest>
      <Telemetry />
    </RequireAuth>
  )
}

export default App
