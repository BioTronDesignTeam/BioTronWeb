import { useState } from 'react'
import TopBar from './components/TopBar'
import { type ViewMode } from './components/ModeToggle'

const MACHINES = ['exo-001', 'exo-002', 'exo-003']

function App() {
  const [machine, setMachine] = useState(MACHINES[0])
  const [mode, setMode] = useState<ViewMode>('live')

  return (
    <div className="min-h-screen bg-slate-950 text-slate-100">
      <TopBar
        machines={MACHINES}
        selectedMachine={machine}
        onSelectMachine={setMachine}
        mode={mode}
        onModeChange={setMode}
      />
      <main className="mx-auto max-w-7xl px-6 py-10">
        <p className="text-sm text-slate-400">
          {mode === 'live' ? 'Live' : 'Historical'} view — <span className="text-slate-200">{machine}</span>
        </p>
      </main>
    </div>
  )
}

export default App
