import { useEffect, useRef, type ReactNode } from 'react';
import { createPortal } from 'react-dom';

interface ModalProps {
  title: string;
  children: ReactNode;
  onClose: () => void;
  wide?: boolean;
}

/**
 * The page lock is shared by every open modal and counted. Each modal used to
 * save the page's styles when it opened and put them back when it closed. With
 * two open at once, such as the cancel confirmation over the event details,
 * the second one saved the *locked* styles. When both closed together it
 * restored those last, and the page stayed locked for good: invisible on the
 * calendar, which never scrolls as a page, and fatal on Manage, which does.
 * Now the first modal to open locks the page and the last to close unlocks it.
 */
let openModals = 0;
let restorePage: (() => void) | undefined;

function lockPage() {
  if (openModals++ === 0) {
    const root = document.documentElement.style;
    const body = document.body.style;
    const appRoot = document.getElementById('root');
    const previous = { overflow: root.overflow, position: body.position, top: body.top, width: body.width };
    // overflow:hidden alone does not hold on iOS Safari, which keeps scrolling
    // the page behind the sheet. Pinning the body at its current offset does,
    // as long as that offset is restored on the way out.
    const scrollY = window.scrollY;
    root.overflow = 'hidden';
    body.position = 'fixed';
    body.top = `-${scrollY}px`;
    body.width = '100%';
    appRoot?.setAttribute('inert', '');
    restorePage = () => {
      root.overflow = previous.overflow;
      body.position = previous.position;
      body.top = previous.top;
      body.width = previous.width;
      window.scrollTo(0, scrollY);
      appRoot?.removeAttribute('inert');
    };
  }
  let released = false;
  return () => {
    if (released) return;
    released = true;
    if (--openModals === 0) {
      restorePage?.();
      restorePage = undefined;
    }
  };
}

export function Modal({ title, children, onClose, wide = false }: ModalProps) {
  const dialogRef = useRef<HTMLElement>(null);
  const closeRef = useRef<HTMLButtonElement>(null);
  const onCloseRef = useRef(onClose);

  // Writing this during render made the mutation visible to render work React
  // is free to replay or throw away. It belongs in a commit-phase effect.
  useEffect(() => {
    onCloseRef.current = onClose;
  });

  useEffect(() => {
    const previousFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const handleKeys = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        onCloseRef.current();
        return;
      }
      if (event.key !== 'Tab' || !dialogRef.current) return;
      const focusable = [...dialogRef.current.querySelectorAll<HTMLElement>(
        'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
      )].filter((element) => !element.hasAttribute('hidden'));
      if (focusable.length === 0) return;
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };
    const unlockPage = lockPage();
    window.addEventListener('keydown', handleKeys);
    closeRef.current?.focus();
    return () => {
      unlockPage();
      window.removeEventListener('keydown', handleKeys);
      previousFocus?.focus();
    };
  }, []);

  return createPortal(
    <div className="fixed inset-0 z-50 flex items-end justify-center p-0 sm:items-center sm:p-6">
      {/* The backdrop is a real close control rather than a div with a click
          handler, so it needs no keyboard shim. It dismisses on click, not
          mousedown: a text selection that starts inside the dialog and drags
          onto the backdrop used to close it mid-gesture. It sits outside the
          focus trap; Escape is the keyboard route. */}
      <button
        type="button"
        aria-label="Close"
        onClick={onClose}
        className="absolute inset-0 cursor-default bg-ink/65 backdrop-blur-sm dark:bg-black/70"
      />
      <section
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby="modal-title"
        className={`relative max-h-[92dvh] sm:max-h-[calc(100dvh-3rem)] w-full overflow-y-auto overscroll-contain rounded-t-3xl border border-soft/40 bg-white p-5 pb-[calc(1.25rem+env(safe-area-inset-bottom,0px))] text-ink sm:rounded-3xl sm:p-7 sm:pb-7 dark:border-line-strong dark:bg-surface dark:text-white ${wide ? 'sm:max-w-4xl' : 'sm:max-w-xl'}`}
      >
        <header className="mb-5 flex items-center justify-between gap-4">
          <h2 id="modal-title" className="min-w-0 break-words text-xl font-semibold tracking-tight">{title}</h2>
          <button ref={closeRef} type="button" onClick={onClose} className="grid size-11 shrink-0 place-items-center rounded-full border border-ink/15 text-xl hover:bg-soft/30 dark:border-line-strong dark:hover:bg-white/10" aria-label="Close">
            ×
          </button>
        </header>
        {children}
      </section>
    </div>,
    document.body,
  );
}
