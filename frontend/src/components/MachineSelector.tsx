import { useEffect, useRef, useState } from 'react'

interface MachineSelectorProps {
  machines: string[]
  selected: string
  onSelect: (id: string) => void
}

export default function MachineSelector({ machines, selected, onSelect }: MachineSelectorProps) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const onPointerDown = (e: PointerEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false)
    }
    document.addEventListener('pointerdown', onPointerDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('pointerdown', onPointerDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-haspopup="listbox"
        aria-expanded={open}
        className="flex items-center gap-2 rounded-lg bg-slate-800/80 px-3 py-2 text-sm font-medium text-slate-100 ring-1 ring-white/10 transition hover:bg-slate-700/80"
      >
        <span className="h-2 w-2 rounded-full bg-emerald-400" />
        <span>{selected}</span>
        <svg
          className={`h-4 w-4 text-slate-400 transition-transform ${open ? 'rotate-180' : ''}`}
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
          className="absolute left-0 z-20 mt-2 min-w-full overflow-hidden rounded-lg bg-slate-800 py-1 shadow-xl ring-1 ring-white/10"
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
                  className={`flex w-full items-center gap-2 px-3 py-2 text-left text-sm transition hover:bg-slate-700 ${
                    active ? 'text-white' : 'text-slate-300'
                  }`}
                >
                  <span className={`h-2 w-2 rounded-full ${active ? 'bg-emerald-400' : 'bg-slate-600'}`} />
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
