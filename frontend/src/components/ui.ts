// Shared control styling, so a button's look is described once instead of
// being retyped as a fifteen-class string at every call site.
const button = 'inline-flex items-center justify-center rounded-lg text-sm font-medium transition';

const outline =
  'border border-slate-300 text-slate-700 hover:bg-slate-100 dark:border-white/10 dark:text-slate-200 dark:hover:bg-slate-800';
const danger =
  'border border-red-300 text-red-700 hover:bg-red-50 dark:border-red-800 dark:text-red-300 dark:hover:bg-red-950/40';
const solid =
  'bg-slate-900 text-white hover:bg-slate-800 dark:bg-slate-100 dark:text-slate-900 dark:hover:bg-white';

export const primaryButton = `${button} ${solid} px-4 py-2.5`;
export const compactPrimaryButton = `${button} ${solid} px-4 py-2 disabled:opacity-50`;
export const secondaryButton = `${button} ${outline} px-4 py-2.5`;
export const compactSecondaryButton = `${button} ${outline} px-3 py-2`;
export const dangerButton = `${button} ${danger} px-3 py-2`;
export const compactDangerButton = `${button} ${danger} px-3 py-1.5`;

export const selectControl = `w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 disabled:opacity-50 dark:border-white/10 dark:bg-slate-800 dark:text-slate-100`;

export const panel =
  'overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm dark:border-white/10 dark:bg-slate-900';
export const panelHeader = 'border-b border-slate-200 px-4 py-4 sm:px-6 dark:border-white/10';
export const panelEmpty = 'px-4 py-10 text-sm text-slate-500 sm:px-6 dark:text-slate-400';
export const panelHeading = 'text-base font-semibold text-slate-900 dark:text-slate-100';
export const mutedText = 'text-sm text-slate-500 dark:text-slate-400';
export const divided = 'divide-y divide-slate-200 dark:divide-white/10';
