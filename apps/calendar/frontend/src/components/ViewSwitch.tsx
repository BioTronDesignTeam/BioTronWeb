import type { CalendarView } from '../date';

interface ViewSwitchProps {
  view: CalendarView;
  onChange: (view: CalendarView) => void;
}

const views: { value: CalendarView; label: string }[] = [
  { value: 'day', label: 'Day' },
  { value: 'week', label: 'Week' },
  { value: 'month', label: 'Month' },
];

/** Day, Week, or Month. One is always pressed. */
export function ViewSwitch({ view, onChange }: ViewSwitchProps) {
  return (
    <fieldset className="flex min-w-0 rounded-full border border-ink/15 p-1 dark:border-line-strong">
      <legend className="sr-only">Calendar view</legend>
      {views.map(({ value, label }) => (
        <button
          key={value}
          type="button"
          aria-pressed={view === value}
          onClick={() => onChange(value)}
          className={`min-h-9 flex-1 rounded-full px-3.5 text-sm font-semibold ${view === value ? 'bg-deep text-white dark:bg-brand' : 'hover:bg-soft/30 dark:hover:bg-white/10'}`}
        >
          {label}
        </button>
      ))}
    </fieldset>
  );
}
