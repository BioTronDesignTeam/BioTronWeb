import { AccountControls, Brand } from '@biotron/style';

export function Header() {
  return (
    <header className="sticky top-0 z-40 border-b border-ink/10 bg-white/90 pt-[env(safe-area-inset-top,0px)] backdrop-blur-xl dark:border-line-strong dark:bg-surface/90">
      <div className="mx-auto flex min-h-16 max-w-[1200px] items-center gap-3 px-4 sm:px-6 lg:px-8">
        <div className="flex min-h-11 min-w-0 items-center gap-3">
          <Brand className="w-28" />
          <span className="hidden h-6 w-px bg-ink/15 sm:block dark:bg-white/20" aria-hidden="true" />
          <span className="hidden truncate text-sm font-semibold tracking-wide sm:block">Sprinter</span>
        </div>
        <AccountControls className="ml-auto" />
      </div>
    </header>
  );
}
