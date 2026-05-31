export type ViewMode = 'live' | 'historical'

interface ModeToggleProps {
  mode: ViewMode
  onModeChange: (mode: ViewMode) => void
}

const OPTIONS: { value: ViewMode; label: string }[] = [
  { value: 'live', label: 'Live' },
  { value: 'historical', label: 'Historical' },
]

export default function ModeToggle({ mode, onModeChange }: ModeToggleProps) {
  const activeIndex = OPTIONS.findIndex((o) => o.value === mode)

  return (
    <div
      role="tablist"
      aria-label="Telemetry view mode"
      className="relative inline-flex items-center rounded-full bg-slate-800/80 p-1 ring-1 ring-white/10"
    >
      {/* sliding highlight — translateX by one button width (w-28 = 7rem) */}
      <span
        aria-hidden
        className="absolute top-1 bottom-1 left-1 w-28 rounded-full bg-slate-100 shadow transition-transform duration-300 ease-out"
        style={{ transform: `translateX(${activeIndex * 7}rem)` }}
      />
      {OPTIONS.map((o) => {
        const active = o.value === mode
        return (
          <button
            key={o.value}
            type="button"
            role="tab"
            aria-selected={active}
            onClick={() => onModeChange(o.value)}
            className={`relative z-10 w-28 rounded-full px-4 py-1.5 text-sm font-medium transition-colors ${
              active ? 'text-slate-900' : 'text-slate-300 hover:text-white'
            }`}
          >
            {o.label}
          </button>
        )
      })}
    </div>
  )
}
