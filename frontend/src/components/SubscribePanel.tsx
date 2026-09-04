import { useMemo, useState } from 'react';
import { feedURL, webcalURL } from '../api';
import type { Scope } from '../types';
import { Modal } from './Modal';

interface SubscribePanelProps {
  scopes: Scope[];
  onClose: () => void;
}

export function SubscribePanel({ scopes, onClose }: SubscribePanelProps) {
  const [copied, setCopied] = useState('');
  const sorted = useMemo(() => {
    const root = scopes.find((scope) => scope.kind === 'TEAM');
    const projects = scopes.filter((scope) => scope.kind === 'PROJECT');
    const subteams = scopes.filter((scope) => scope.kind === 'SUBTEAM');
    return { root, projects, subteams };
  }, [scopes]);

  async function copy(id?: string) {
    await navigator.clipboard.writeText(feedURL(id));
    setCopied(id || 'all');
    window.setTimeout(() => setCopied(''), 1800);
  }

  const row = (label: string, detail: string, id?: string) => (
    <div className="flex flex-col gap-3 rounded-2xl border border-ink/10 p-4 dark:border-white/10 sm:flex-row sm:items-center">
      <div className="min-w-0 flex-1">
        <p className="font-semibold">{label}</p>
        <p className="mt-1 text-sm text-ink/60 dark:text-white/60">{detail}</p>
      </div>
      <div className="flex gap-2">
        <button type="button" onClick={() => void copy(id)} className="min-h-11 rounded-full border border-ink/15 px-4 text-sm font-semibold hover:bg-soft/25 dark:border-white/20 dark:hover:bg-white/10">
          {copied === (id || 'all') ? 'Copied' : 'Copy URL'}
        </button>
        <a href={webcalURL(id)} className="inline-flex min-h-11 items-center rounded-full bg-deep px-4 text-sm font-semibold text-white hover:bg-brand">Subscribe</a>
      </div>
    </div>
  );

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
              {sorted.subteams.filter((scope) => scope.parent_id === project.id).map((subteam) => (
                <div key={subteam.id} className="sm:ml-6">{row(subteam.name, `Only ${subteam.name} subteam events.`, subteam.id)}</div>
              ))}
            </div>
          </section>
        ))}
      </div>
      <p className="mt-6 text-xs leading-5 text-ink/50 dark:text-white/50">Calendar apps control how quickly subscription updates appear. Event changes and cancellations remain available in the feed so Apple Calendar, Google Calendar, and Outlook can reconcile them.</p>
    </Modal>
  );
}
