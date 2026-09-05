import { Brand, ThemeToggle, UserMenu } from '@biotron/style'
import MachineSelector from './MachineSelector'
import ModeToggle, { type ViewMode } from './ModeToggle'
import { useAuth } from '../auth/context'

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
  const { state, logout } = useAuth()
  const operator = state.status === 'authed' ? state.operator : undefined

  return (
    <header className="sticky top-0 z-30 border-b border-[#aedbfc] bg-white/90 backdrop-blur dark:border-[#aedbfc]/20 dark:bg-[#160b6c]/90">
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

        {/* Right: theme toggle + user menu */}
        <div className="flex items-center gap-2 justify-self-end">
          <ThemeToggle />
          <UserMenu
            user={operator ? {
              name: operator.name,
              login: operator.login,
              avatarUrl: operator.avatar_url,
            } : undefined}
            onLogout={logout}
          />
        </div>
      </div>
    </header>
  )
}
