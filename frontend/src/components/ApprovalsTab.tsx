import type { AccessRequest } from '../api';
import {
  divided,
  mutedText,
  panel,
  panelEmpty,
  panelHeader,
  panelHeading,
  primaryButton,
  secondaryButton,
} from './ui';

export default function ApprovalsTab({
  pending,
  onApprove,
  onDeny,
}: {
  pending: AccessRequest[];
  onApprove: (id: string) => void;
  onDeny: (id: string) => void;
}) {
  return (
    <section className={panel}>
      <div className={panelHeader}>
        <h2 className={panelHeading}>Pending requests</h2>
      </div>
      {pending.length === 0 && <p className={panelEmpty}>Nothing waiting for approval.</p>}
      <ul className={divided}>
        {pending.map((request) => (
          <li
            key={request.id}
            className="flex flex-col gap-4 px-4 py-5 sm:flex-row sm:items-center sm:justify-between sm:px-6"
          >
            <div className="min-w-0">
              <div className="text-base font-medium break-words text-slate-900 dark:text-slate-100">
                @{request.requester_login} → {request.app_name} / {request.permission_label}
              </div>
              <div className={`mt-1 ${mutedText}`}>{new Date(request.created_at).toLocaleString()}</div>
            </div>
            <div className="grid grid-cols-2 gap-2 sm:flex">
              <button type="button" onClick={() => onApprove(request.id)} className={primaryButton}>
                Approve
              </button>
              <button type="button" onClick={() => onDeny(request.id)} className={secondaryButton}>
                Deny
              </button>
            </div>
          </li>
        ))}
      </ul>
    </section>
  );
}
