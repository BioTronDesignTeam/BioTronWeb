import { useDismissable } from '../hooks/useDismissable'

interface MachineSelectorProps {
  machines: string[]
  selected: string
  onSelect: (id: string) => void
}

export default function MachineSelector({ machines, selected, onSelect }: MachineSelectorProps) {
  const { open, setOpen, ref } = useDismissable<HTMLDivElement>()

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-haspopup="listbox"
        aria-expanded={open}
        className="flex items-center gap-2 rounded-lg bg-white px-3 py-2 text-sm font-medium text-slate-900 ring-1 ring-slate-300 transition hover:bg-slate-100 dark:bg-slate-800/80 dark:text-slate-100 dark:ring-white/10 dark:hover:bg-slate-700/80"
      >
        <span className="h-2 w-2 rounded-full bg-emerald-400" />
        <span>{selected}</span>
        <svg
          className={`h-4 w-4 text-slate-500 transition-transform dark:text-slate-400 ${open ? 'rotate-180' : ''}`}
          viewBox="0 0 20 20"
          fill="currentColor"
          aria-hidden
        >
          <path
            fillRule="evenodd"
            d="M5.23 7.21a.75.75 0 0 1 1.06.02L10 11.06l3.71-3.83a.75.75 0 1 1 1.08 1.04l-4.25 4.39a.75.75 0 0 1-1.08 0L5.21 8.27a.75.75 0 0 1 .02-1.06Z"
            clipRule="evenodd"
          />
        </svg>
      </button>

      {open && (
        <ul
          role="listbox"
          className="absolute left-0 z-20 mt-2 min-w-full overflow-hidden rounded-lg bg-white py-1 shadow-xl ring-1 ring-slate-200 dark:bg-slate-800 dark:ring-white/10"
        >
          {machines.map((id) => {
            const active = id === selected
            return (
              <li key={id} role="option" aria-selected={active}>
                <button
                  type="button"
                  onClick={() => {
                    onSelect(id)
                    setOpen(false)
                  }}
                  className={`flex w-full items-center gap-2 px-3 py-2 text-left text-sm transition hover:bg-slate-100 dark:hover:bg-slate-700 ${
                    active ? 'text-slate-900 dark:text-white' : 'text-slate-700 dark:text-slate-300'
                  }`}
                >
                  <span className={`h-2 w-2 rounded-full ${active ? 'bg-emerald-400' : 'bg-slate-300 dark:bg-slate-600'}`} />
                  {id}
                </button>
              </li>
            )
          })}
        </ul>
      )}
    </div>
  )
}
