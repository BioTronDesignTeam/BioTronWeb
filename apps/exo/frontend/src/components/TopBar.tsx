import { AccountControls, Brand } from '@biotron/style'
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
    <header className="sticky top-0 z-30 border-b border-[#aedbfc] bg-white/90 backdrop-blur dark:border-white/[0.14] dark:bg-surface/90">
      <div className="grid grid-cols-3 items-center px-6 py-3">
        {/* Left: machine selector */}
        <div className="flex items-center gap-3 justify-self-start">
          {/* The wordmark at the width every other BioTron header uses. */}
          <Brand className="w-28" />
          <MachineSelector machines={machines} selected={selectedMachine} onSelect={onSelectMachine} />
        </div>

        {/* Center: Live / Historical toggle */}
        <div className="justify-self-center">
          <ModeToggle mode={mode} onModeChange={onModeChange} />
        </div>

        {/* Right: theme toggle + account menu */}
        <AccountControls className="justify-self-end" />
      </div>
    </header>
  )
}
