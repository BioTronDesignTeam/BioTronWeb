interface MachineSelectorProps {
  machines: string[];
  selected: string;
  onSelect: (id: string) => void;
}

export default function MachineSelector({ machines, selected, onSelect }: MachineSelectorProps) {
  return (
    <label className="relative inline-flex min-w-0 items-center">
      <span className="sr-only">Machine</span>
      <span className="pointer-events-none absolute left-3 h-2 w-2 rounded-full bg-emerald-400" />
      <select
        value={selected}
        onChange={(e) => onSelect(e.target.value)}
        className="max-w-full min-w-0 appearance-none truncate rounded-lg bg-white py-2 pr-3 pl-7 text-sm font-medium text-slate-900 ring-1 ring-slate-300 transition hover:bg-slate-100 sm:pr-8 dark:bg-surface-2/80 dark:text-white dark:ring-white/10 dark:hover:bg-surface-2 pointer-coarse:min-h-11"
      >
        {machines.map((id) => (
          <option key={id} value={id}>
            {id}
          </option>
        ))}
      </select>
    </label>
  );
}
