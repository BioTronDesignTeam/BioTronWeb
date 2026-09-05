import { useEffect, useMemo, useRef, useState } from 'react';
import type { Scope } from '../types';

interface ScopeFilterProps {
  scopes: Scope[];
  selected: string[];
  onChange: (scopeIds: string[]) => void;
}

interface ScopeNode {
  scope: Scope;
  children: ScopeNode[];
}

/** Builds the team / project / subteam tree the filter draws. */
function buildTree(scopes: Scope[]): ScopeNode[] {
  const nodes = new Map<string, ScopeNode>(scopes.map((scope) => [scope.id, { scope, children: [] }]));
  const roots: ScopeNode[] = [];
  for (const scope of scopes) {
    const node = nodes.get(scope.id);
    if (!node) continue;
    const parent = scope.parent_id ? nodes.get(scope.parent_id) : undefined;
    if (parent) parent.children.push(node);
    else roots.push(node);
  }
  const byName = (a: ScopeNode, b: ScopeNode) => a.scope.name.localeCompare(b.scope.name);
  const sort = (list: ScopeNode[]) => {
    list.sort(byName);
    for (const node of list) sort(node.children);
  };
  sort(roots);
  return roots;
}

function subtreeIds(node: ScopeNode): string[] {
  return [node.scope.id, ...node.children.flatMap(subtreeIds)];
}

/**
 * A checkbox tree of scopes. An empty selection means every event, which is
 * both the resting state and what "Clear" returns to; ticking a scope ticks
 * its whole subtree, so choosing Exo also brings in Exo's subteams.
 */
export function ScopeFilter({ scopes, selected, onChange }: ScopeFilterProps) {
  const [open, setOpen] = useState(false);
  const container = useRef<HTMLDivElement>(null);
  const tree = useMemo(() => buildTree(scopes), [scopes]);
  const chosen = useMemo(() => new Set(selected), [selected]);

  // The panel closes on Escape and on a click anywhere outside it, so it never
  // sits open over the grid the user went back to reading.
  useEffect(() => {
    if (!open) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setOpen(false);
    };
    const onPointer = (event: MouseEvent) => {
      if (!container.current?.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener('keydown', onKey);
    document.addEventListener('mousedown', onPointer);
    return () => {
      document.removeEventListener('keydown', onKey);
      document.removeEventListener('mousedown', onPointer);
    };
  }, [open]);

  function toggle(ids: string[], checked: boolean) {
    const next = new Set(chosen);
    for (const id of ids) {
      if (checked) next.add(id);
      else next.delete(id);
    }
    onChange([...next]);
  }

  return (
    <div ref={container} className="relative self-start sm:self-auto">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        aria-expanded={open}
        aria-label={selected.length === 0 ? 'Filter calendars' : `Filter calendars, ${selected.length} selected`}
        className="relative grid size-11 place-items-center rounded-full text-ink hover:bg-soft/25 dark:text-white dark:hover:bg-white/10"
      >
        <svg width="20" height="20" viewBox="0 0 20 20" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round">
          <line x1="3" y1="6" x2="17" y2="6" />
          <line x1="3" y1="10" x2="17" y2="10" />
          <line x1="3" y1="14" x2="17" y2="14" />
        </svg>
        {selected.length > 0 && (
          <span className="absolute right-0 top-0 grid h-5 min-w-5 place-items-center rounded-full bg-brand px-1 text-[11px] font-bold leading-none text-white dark:bg-soft dark:text-ink">
            {selected.length}
          </span>
        )}
      </button>

      {open && (
        <div className="absolute right-0 top-full z-30 mt-2 max-h-[22rem] w-72 overflow-y-auto rounded-2xl border border-ink/10 bg-white p-2 shadow-xl dark:border-white/15 dark:bg-ink">
          <div className="flex items-center justify-between px-2 py-1">
            <span className="text-xs font-bold uppercase tracking-[0.16em] text-ink/50 dark:text-white/50">Calendars</span>
            <button
              type="button"
              onClick={() => onChange([])}
              disabled={selected.length === 0}
              className="rounded-full px-2 py-1 text-xs font-semibold text-brand hover:bg-soft/30 disabled:opacity-40 dark:text-soft dark:hover:bg-white/10"
            >
              Clear
            </button>
          </div>
          <ul>
            {tree.map((node) => <ScopeBranch key={node.scope.id} node={node} depth={0} chosen={chosen} onToggle={toggle} />)}
          </ul>
        </div>
      )}
    </div>
  );
}

interface ScopeBranchProps {
  node: ScopeNode;
  depth: number;
  chosen: Set<string>;
  onToggle: (scopeIds: string[], checked: boolean) => void;
}

function ScopeBranch({ node, depth, chosen, onToggle }: ScopeBranchProps) {
  const ids = subtreeIds(node);
  const checkedCount = ids.filter((id) => chosen.has(id)).length;
  const checked = checkedCount === ids.length;
  // A half-filled parent has to say so, or a project with one subteam ticked
  // looks exactly like a project with none.
  const partial = checkedCount > 0 && !checked;

  return (
    <li>
      <ScopeRow
        label={node.scope.name}
        depth={depth}
        bold={depth === 0}
        checked={checked}
        partial={partial}
        onChange={(next) => onToggle(ids, next)}
      />
      {node.children.length > 0 && (
        <ul>
          {/* A scope that has children can still own events of its own: the
              ones for everybody in it rather than for one subteam. "General"
              is that row, so Exo's all-hands meetings can be chosen without
              its subteams coming along. */}
          <ScopeRow
            label="General"
            depth={depth + 1}
            checked={chosen.has(node.scope.id)}
            partial={false}
            onChange={(next) => onToggle([node.scope.id], next)}
          />
          {node.children.map((child) => (
            <ScopeBranch key={child.scope.id} node={child} depth={depth + 1} chosen={chosen} onToggle={onToggle} />
          ))}
        </ul>
      )}
    </li>
  );
}

interface ScopeRowProps {
  label: string;
  depth: number;
  bold?: boolean;
  checked: boolean;
  partial: boolean;
  onChange: (checked: boolean) => void;
}

function ScopeRow({ label, depth, bold = false, checked, partial, onChange }: ScopeRowProps) {
  const box = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (box.current) box.current.indeterminate = partial;
  }, [partial]);
  return (
    <label
      className="flex min-h-10 cursor-pointer items-center gap-2 rounded-xl px-2 text-sm hover:bg-soft/25 dark:hover:bg-white/10"
      style={{ paddingLeft: `${0.5 + depth * 1.1}rem` }}
    >
      <input
        ref={box}
        type="checkbox"
        checked={checked}
        onChange={(event) => onChange(event.target.checked)}
        className="size-4 accent-brand"
      />
      <span className={`truncate ${bold ? 'font-semibold' : ''}`}>{label}</span>
    </label>
  );
}
