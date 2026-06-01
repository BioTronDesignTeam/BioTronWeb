import type { ReactNode } from 'react'

export interface SegmentOption<T extends string> {
  value: T
  label?: string
  icon?: ReactNode
  ariaLabel?: string
}

interface SegmentedToggleProps<T extends string> {
  options: SegmentOption<T>[]
  value: T
  onChange: (value: T) => void
  ariaLabel: string
  /** width of each (equal-width) segment, in rem */
  segmentRem: number
}

export default function SegmentedToggle<T extends string>({
  options,
  value,
  onChange,
  ariaLabel,
  segmentRem,
}: SegmentedToggleProps<T>) {
  const activeIndex = Math.max(
    0,
    options.findIndex((o) => o.value === value),
  )
  const isBinary = options.length === 2
  // The whole pill is one control: a click anywhere advances to the next option.
  const nextValue = options[(activeIndex + 1) % options.length].value

  return (
    <button
      type="button"
      role={isBinary ? 'switch' : undefined}
      aria-checked={isBinary ? activeIndex === 1 : undefined}
      aria-label={ariaLabel}
      onClick={() => onChange(nextValue)}
      className="group relative inline-flex cursor-pointer items-center rounded-full bg-slate-200/80 p-1 ring-1 ring-slate-300 focus:outline-none focus-visible:ring-2 focus-visible:ring-slate-400 dark:bg-slate-800/80 dark:ring-white/10 dark:focus-visible:ring-slate-500"
    >
      {/* sliding highlight — same width as a segment, translated by the active index */}
      <span
        aria-hidden
        className="absolute top-1 bottom-1 left-1 rounded-full bg-white shadow transition-transform duration-300 ease-out dark:bg-slate-100"
        style={{ width: `${segmentRem}rem`, transform: `translateX(${activeIndex * segmentRem}rem)` }}
      />
      {options.map((o) => {
        const active = o.value === value
        return (
          <span
            key={o.value}
            style={{ width: `${segmentRem}rem` }}
            className={`relative z-10 flex items-center justify-center rounded-full py-1.5 text-sm font-medium ${
              active
                ? 'text-slate-900'
                : 'text-slate-600 group-hover:text-slate-800 dark:text-slate-300 dark:group-hover:text-slate-100'
            }`}
          >
            {o.icon ?? o.label}
          </span>
        )
      })}
    </button>
  )
}
