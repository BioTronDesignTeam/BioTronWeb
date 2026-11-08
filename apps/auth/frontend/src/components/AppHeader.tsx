import { AccountControls, Brand, identityLine } from '@biotron/style';

export default function AppHeader() {
  return (
    <header className="pt-safe sticky top-0 z-30 border-b border-[#aedbfc] bg-white/90 backdrop-blur dark:border-white/[0.14] dark:bg-surface/90">
      <div className="px-page mx-auto flex w-full max-w-6xl flex-wrap items-center justify-between gap-x-4 gap-y-3 py-4">
        {/* The wordmark at the width every other BioTron header uses, with the
            product name beside it. On a phone the wordmark stands alone, as it
            does on the other sites. */}
        <div className="flex min-h-11 min-w-0 items-center gap-3">
          <Brand className="w-28" />
          <span
            className="hidden h-6 w-px bg-[#16033c]/15 sm:block dark:bg-white/20"
            aria-hidden="true"
          />
          {/* The page's one h1, read on a phone where it is not drawn. */}
          <h1 className="sr-only truncate text-sm font-semibold tracking-wide text-[#16033c] sm:not-sr-only sm:block dark:text-white">
            Auth
          </h1>
        </div>
        <AccountControls className="flex-wrap gap-3" detail={identityLine} />
      </div>
    </header>
  );
}
