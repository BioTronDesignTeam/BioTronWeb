import MachineSelector from './MachineSelector'
import ModeToggle, { type ViewMode } from './ModeToggle'

interface TopBarProps {
  machines: string[]
  selectedMachine: string
  onSelectMachine: (id: string) => void
  mode: ViewMode
  onModeChange: (mode: ViewMode) => void
}

export default function TopBar({
  machines,
  selectedMachine,
  onSelectMachine,
  mode,
  onModeChange,
}: TopBarProps) {
  return (
    <header className="sticky top-0 z-30 border-b border-white/10 bg-slate-900/80 backdrop-blur">
      <div className="grid grid-cols-3 items-center px-6 py-3">
        {/* Left: machine selector */}
        <div className="justify-self-start">
          <MachineSelector machines={machines} selected={selectedMachine} onSelect={onSelectMachine} />
        </div>

        {/* Center: Live / Historical toggle */}
        <div className="justify-self-center">
          <ModeToggle mode={mode} onModeChange={onModeChange} />
        </div>

        {/* Right: user */}
        <div className="justify-self-end">
          <button
            type="button"
            aria-label="User menu"
            className="flex h-9 w-9 items-center justify-center rounded-full bg-slate-800 text-slate-300 ring-1 ring-white/10 transition hover:bg-slate-700 hover:text-white"
          >
            <svg viewBox="0 0 24 24" fill="currentColor" className="h-5 w-5" aria-hidden>
              <path d="M12 12a5 5 0 1 0 0-10 5 5 0 0 0 0 10Zm0 2c-4.42 0-8 2.69-8 6v1a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1v-1c0-3.31-3.58-6-8-6Z" />
            </svg>
          </button>
        </div>
      </div>
    </header>
  )
}
