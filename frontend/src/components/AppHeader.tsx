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
    <header className="sticky top-0 z-30 border-b border-[#aedbfc] bg-white/90 backdrop-blur dark:border-[#aedbfc]/20 dark:bg-[#160b6c]/90">
      <div className="mx-auto flex w-full max-w-6xl flex-wrap items-center justify-between gap-x-4 gap-y-3 px-4 py-4 sm:px-6 lg:px-8">
        <div className="flex min-w-0 items-center gap-3">
          <Brand compact />
          <div className="min-w-0">
            <div className="text-base font-semibold text-[#16033c] dark:text-white">BioTron Auth</div>
            <div className="mt-0.5 text-sm text-[#3050b0] dark:text-[#aedbfc]">
              Request and approve access to team tools
            </div>
          </div>
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
