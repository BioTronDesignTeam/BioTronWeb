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
 * A checkbox tree of scopes, drawn inline in the sidebar. An empty selection
 * means every event, which is both the resting state and what "Clear" returns
 * to; ticking a scope ticks its whole subtree, so choosing Exo also brings in
 * Exo's subteams.
 */
export function ScopeFilter({ scopes, selected, onChange }: ScopeFilterProps) {
  const tree = useMemo(() => buildTree(scopes), [scopes]);
  const chosen = useMemo(() => new Set(selected), [selected]);

  function toggle(ids: string[], checked: boolean) {
    const next = new Set(chosen);
    for (const id of ids) {
      if (checked) next.add(id);
      else next.delete(id);
    }
    onChange([...next]);
  }

  return (
    <div>
      <div className="flex items-center justify-between px-2 py-1">
        <span className="text-xs font-bold uppercase tracking-[0.16em] text-ink/50 dark:text-muted">Calendars</span>
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
  const [expanded, setExpanded] = useState(true);
  const ids = subtreeIds(node);
  const checkedCount = ids.filter((id) => chosen.has(id)).length;
  const checked = checkedCount === ids.length;
  // A half-filled parent has to say so, or a project with one subteam ticked
  // looks exactly like a project with none. It matters most when the branch
  // is folded and the parent is all there is to see.
  const partial = checkedCount > 0 && !checked;
  const hasChildren = node.children.length > 0;

  return (
    <li>
      <ScopeRow
        label={node.scope.name}
        depth={depth}
        bold={depth === 0}
        checked={checked}
        partial={partial}
        onChange={(next) => onToggle(ids, next)}
        expanded={hasChildren ? expanded : undefined}
        onExpand={() => setExpanded(!expanded)}
      />
      {hasChildren && expanded && (
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
  /** Set on a row with children. Leaf rows leave it out and keep the gap, so every checkbox lines up. */
  expanded?: boolean;
  onExpand?: () => void;
}

function ScopeRow({ label, depth, bold = false, checked, partial, onChange, expanded, onExpand }: ScopeRowProps) {
  const box = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (box.current) box.current.indeterminate = partial;
  }, [partial]);
  return (
    <div className="flex items-center" style={{ paddingLeft: `${depth * 1.1}rem` }}>
      {expanded === undefined ? <span className="size-7 shrink-0" aria-hidden="true" /> : (
        <button
          type="button"
          onClick={onExpand}
          aria-expanded={expanded}
          aria-label={`${expanded ? 'Collapse' : 'Expand'} ${label}`}
          className="grid size-7 shrink-0 place-items-center rounded-full text-ink/55 hover:bg-soft/30 dark:text-muted dark:hover:bg-white/10"
        >
          <svg width="12" height="12" viewBox="0 0 12 12" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className={expanded ? 'rotate-90' : ''}>
            <polyline points="4,2 8,6 4,10" />
          </svg>
        </button>
      )}
      <label className="flex min-h-10 min-w-0 flex-1 cursor-pointer items-center gap-2 rounded-xl px-1.5 text-sm hover:bg-soft/25 dark:hover:bg-white/10">
        <input
          ref={box}
          type="checkbox"
          checked={checked}
          onChange={(event) => onChange(event.target.checked)}
          className="size-4 shrink-0 accent-brand"
        />
        <span className={`truncate ${bold ? 'font-semibold' : ''}`}>{label}</span>
      </label>
    </div>
  );
}
