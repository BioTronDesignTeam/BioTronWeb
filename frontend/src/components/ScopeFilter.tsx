import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
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
  const panel = useRef<HTMLDivElement>(null);
  const [placement, setPlacement] = useState({ top: 0, right: 0, maxHeight: 0 });
  const tree = useMemo(() => buildTree(scopes), [scopes]);
  const chosen = useMemo(() => new Set(selected), [selected]);

  // The calendar card clips its own corners with overflow-hidden, which also
  // clipped this panel whenever the card was shorter than the panel: on a phone
  // in a quiet month the list was cut off mid-tree. It renders in a portal
  // instead, positioned against the button rather than nested inside it.
  const place = useCallback(() => {
    const button = container.current?.getBoundingClientRect();
    if (!button) return;
    const top = button.bottom + 8;
    setPlacement({
      top,
      right: Math.max(8, window.innerWidth - button.right),
      maxHeight: Math.max(160, window.innerHeight - top - 16),
    });
  }, []);

  useLayoutEffect(() => {
    if (open) place();
  }, [open, place]);

  useEffect(() => {
    if (!open) return;
    window.addEventListener('resize', place);
    window.addEventListener('scroll', place, true);
    return () => {
      window.removeEventListener('resize', place);
      window.removeEventListener('scroll', place, true);
    };
  }, [open, place]);

  // The panel closes on Escape and on a click anywhere outside it, so it never
  // sits open over the grid the user went back to reading.
  useEffect(() => {
    if (!open) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setOpen(false);
    };
    const onPointer = (event: MouseEvent) => {
      const target = event.target as Node;
      if (container.current?.contains(target) || panel.current?.contains(target)) return;
      setOpen(false);
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
    <div ref={container} className="relative shrink-0">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        aria-expanded={open}
        aria-label={selected.length === 0 ? 'Filter calendars' : `Filter calendars, ${selected.length} selected`}
        className="relative grid size-12 place-items-center rounded-full text-ink hover:bg-soft/25 dark:text-white dark:hover:bg-white/10"
      >
        {/* Döner, not hamburger: three centred lines of decreasing length is
            the filter mark. Equal lines would read as a navigation menu. */}
        <svg width="26" height="26" viewBox="0 0 26 26" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
          <line x1="4" y1="7" x2="22" y2="7" />
          <line x1="7" y1="13" x2="19" y2="13" />
          <line x1="10" y1="19" x2="16" y2="19" />
        </svg>
        {selected.length > 0 && (
          <span className="absolute right-0.5 top-0.5 grid h-5 min-w-5 place-items-center rounded-full bg-brand px-1 text-[11px] font-bold leading-none text-white">
            {selected.length}
          </span>
        )}
      </button>

      {open && createPortal(
        <div
          ref={panel}
          style={{ top: placement.top, right: placement.right, maxHeight: placement.maxHeight }}
          className="fixed z-50 w-72 max-w-[calc(100vw-1rem)] overflow-y-auto rounded-2xl border border-ink/10 bg-white p-2 shadow-xl dark:border-line-strong dark:bg-surface"
        >
          <div className="flex items-center justify-between px-2 py-1">
            <span className="text-xs font-bold uppercase tracking-[0.16em] text-ink/50 dark:text-white/50">Calendars</span>
            <button
              type="button"
              onClick={() => onChange([])}
              disabled={selected.length === 0}
              className="rounded-full px-2 py-1 text-xs font-semibold text-brand hover:bg-soft/30 disabled:opacity-40 dark:text-link dark:hover:bg-white/10"
            >
              Clear
            </button>
          </div>
          <ul>
            {tree.map((node) => <ScopeBranch key={node.scope.id} node={node} depth={0} chosen={chosen} onToggle={toggle} />)}
          </ul>
        </div>,
        document.body,
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
