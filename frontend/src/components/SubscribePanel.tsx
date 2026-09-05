import { useMemo, useState } from 'react';
import { feedURL, webcalURL } from '../api';
import type { Scope } from '../types';
import { Modal } from './Modal';

interface SubscribePanelProps {
  scopes: Scope[];
  onClose: () => void;
}

type CopyResult = { key: string; copied: boolean };

export function SubscribePanel({ scopes, onClose }: SubscribePanelProps) {
  const [result, setResult] = useState<CopyResult>();
  const sorted = useMemo(() => {
    const projects: Scope[] = [];
    const subteamsByProject = new Map<string, Scope[]>();
    let root: Scope | undefined;
    for (const scope of scopes) {
      if (scope.kind === 'TEAM') root = scope;
      else if (scope.kind === 'PROJECT') projects.push(scope);
      else if (scope.parent_id) subteamsByProject.set(scope.parent_id, [...(subteamsByProject.get(scope.parent_id) || []), scope]);
    }
    return { root, projects, subteamsByProject };
  }, [scopes]);

  // navigator.clipboard is missing entirely on a non-secure origin — which is
  // exactly what a phone on the LAN gets at http://192.168.x.x:5176 — and can
  // reject on permission elsewhere. Swallowing that left the button doing
  // nothing at all, with nothing on the clipboard and no message.
  async function copy(key: string, url: string) {
    try {
      if (!navigator.clipboard?.writeText) throw new Error('clipboard unavailable');
      await navigator.clipboard.writeText(url);
      setResult({ key, copied: true });
    } catch {
      setResult({ key, copied: false });
    }
    window.setTimeout(() => setResult(undefined), 5000);
  }

  const row = (label: string, detail: string, id?: string) => {
    const key = id || 'all';
    const url = feedURL(id);
    const outcome = result?.key === key ? result : undefined;
    return (
      <div className="rounded-2xl border border-ink/10 p-4 dark:border-white/10">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
          <div className="min-w-0 flex-1">
            <p className="font-semibold">{label}</p>
            <p className="mt-1 text-sm text-ink/60 dark:text-white/60">{detail}</p>
          </div>
          <div className="flex gap-2">
            <button type="button" onClick={() => void copy(key, url)} className="min-h-11 rounded-full border border-ink/15 px-4 text-sm font-semibold hover:bg-soft/25 dark:border-white/20 dark:hover:bg-white/10">
              {outcome?.copied ? 'Copied' : 'Copy URL'}
            </button>
            <a href={webcalURL(id)} className="inline-flex min-h-11 items-center rounded-full bg-deep px-4 text-sm font-semibold text-white hover:bg-brand">Subscribe</a>
          </div>
        </div>
        {/* Always visible, always selectable. webcal: has no handler on Android
            Chrome or most Linux desktops, so the Subscribe button alone can
            leave a phone user with no way to reach the feed at all. */}
        <input
          readOnly
          value={url}
          aria-label={`${label} subscription address`}
          onFocus={(event) => event.currentTarget.select()}
          onClick={(event) => event.currentTarget.select()}
          className="mt-3 min-h-11 w-full select-all rounded-xl border border-ink/10 bg-ink/[0.03] px-3 font-mono text-xs text-ink/70 outline-none focus:border-brand dark:border-line-strong dark:bg-surface-deep dark:text-muted"
        />
        {outcome && (
          <p role="status" className={`mt-2 text-xs ${outcome.copied ? 'text-ink/60 dark:text-white/60' : 'text-red-700 dark:text-red-300'}`}>
            {outcome.copied
              ? 'Address copied. Paste it into "Add calendar by URL" in Google, Apple, or Outlook Calendar.'
              : 'This browser would not let the page use the clipboard. Tap the address above to select it, then copy it yourself.'}
          </p>
        )}
      </div>
    );
  };

  return (
    <Modal title="Choose your calendar" onClose={onClose} wide>
      <p className="mb-6 max-w-2xl text-sm leading-6 text-ink/65 dark:text-white/65">
        Each subscription is independent. Choose only the teamwide, project, or subteam events you want. Subscribe to several without receiving duplicate parent events.
      </p>
      <div className="space-y-6">
        <section>
          <h3 className="mb-2 text-xs font-bold uppercase tracking-[0.16em] text-brand dark:text-soft">Everything</h3>
          {row('All BioTron events', 'Every public event across the team, projects, and subteams.')}
        </section>
        {sorted.root && (
          <section>
            <h3 className="mb-2 text-xs font-bold uppercase tracking-[0.16em] text-brand dark:text-soft">Teamwide</h3>
            {row('Teamwide events', 'Only events assigned to the whole BioTron team.', sorted.root.id)}
          </section>
        )}
        {sorted.projects.map((project) => (
          <section key={project.id}>
            <h3 className="mb-2 text-xs font-bold uppercase tracking-[0.16em] text-brand dark:text-soft">{project.name}</h3>
            <div className="space-y-2">
              {row(`${project.name} project`, `Only project-wide ${project.name} events.`, project.id)}
              {(sorted.subteamsByProject.get(project.id) || []).map((subteam) => (
                <div key={subteam.id} className="sm:ml-6">{row(subteam.name, `Only ${subteam.name} subteam events.`, subteam.id)}</div>
              ))}
            </div>
          </section>
        ))}
      </div>
      <p className="mt-6 text-xs leading-5 text-ink/50 dark:text-white/50">
        Subscribe opens your calendar app directly where the <code>webcal</code> scheme is handled. On Android and most Linux desktops it is not, so use the address instead. Calendar apps control how quickly subscription updates appear; event changes and cancellations remain in the feed so Apple Calendar, Google Calendar, and Outlook can reconcile them.
      </p>
    </Modal>
  );
}
