import { Brand, ThemeToggle, UserMenu } from '@biotron/style';
import type { Me } from '../api';

function roleSuffix(me: Me): string {
  if (me.is_guest) return ' · guest';
  if (me.is_superuser) return ' · superuser';
  if (me.is_manager) return ' · manager';
  if (me.is_staff) return ' · staff';
  return '';
}

export default function AppHeader({ me, onLogout }: { me: Me; onLogout: () => void }) {
  return (
    <header className="sticky top-0 z-30 border-b border-[#aedbfc] bg-white/90 pt-safe backdrop-blur dark:border-white/[0.14] dark:bg-surface/90">
      <div className="mx-auto flex w-full max-w-6xl flex-wrap items-center justify-between gap-x-4 gap-y-3 px-page py-4">
        {/* The wordmark at the width every other BioTron header uses, with the
            product name beside it. On a phone the wordmark stands alone, as it
            does on the other sites. */}
        <div className="flex min-h-11 min-w-0 items-center gap-3">
          <Brand className="w-28" />
          <span className="hidden h-6 w-px bg-[#16033c]/15 sm:block dark:bg-white/20" aria-hidden="true" />
          <span className="hidden truncate text-sm font-semibold tracking-wide text-[#16033c] sm:block dark:text-white">
            Auth
          </span>
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <ThemeToggle />
          <UserMenu
            user={{
              name: me.name,
              login: me.login,
              avatarUrl: me.avatar_url,
              detail: `@${me.login}${roleSuffix(me)}`,
            }}
            onLogout={onLogout}
          />
        </div>
      </div>
    </header>
  );
}
