import { useEffect, useRef, type ReactNode } from 'react';
import { createPortal } from 'react-dom';

interface ModalProps {
  title: string;
  children: ReactNode;
  onClose: () => void;
  wide?: boolean;
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
    const previousOverflow = document.documentElement.style.overflow;
    const previousFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const appRoot = document.getElementById('root');
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
    // overflow:hidden alone does not hold on iOS Safari, which keeps scrolling
    // the page behind the sheet. Pinning the body at its current offset does,
    // as long as that offset is restored on the way out.
    const scrollY = window.scrollY;
    const body = document.body.style;
    const previousBody = { position: body.position, top: body.top, width: body.width };
    document.documentElement.style.overflow = 'hidden';
    body.position = 'fixed';
    body.top = `-${scrollY}px`;
    body.width = '100%';
    appRoot?.setAttribute('inert', '');
    window.addEventListener('keydown', handleKeys);
    closeRef.current?.focus();
    return () => {
      document.documentElement.style.overflow = previousOverflow;
      body.position = previousBody.position;
      body.top = previousBody.top;
      body.width = previousBody.width;
      window.scrollTo(0, scrollY);
      appRoot?.removeAttribute('inert');
      window.removeEventListener('keydown', handleKeys);
      previousFocus?.focus();
    };
  }, []);

  return createPortal(
    // Dismiss on click, not mousedown: a text selection that starts inside the
    // dialog and drags onto the backdrop used to close it mid-gesture.
    <div className="fixed inset-0 z-50 flex items-end justify-center bg-ink/65 p-0 backdrop-blur-sm dark:bg-black/70 sm:items-center sm:p-6" role="presentation" onClick={onClose}>
      <section
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby="modal-title"
        className={`max-h-[92dvh] w-full overflow-y-auto overscroll-contain rounded-t-3xl border border-soft/40 bg-white p-5 pb-[calc(1.25rem+env(safe-area-inset-bottom,0px))] text-ink sm:rounded-3xl sm:p-7 sm:pb-7 dark:border-line-strong dark:bg-surface dark:text-white ${wide ? 'sm:max-w-4xl' : 'sm:max-w-xl'}`}
        onClick={(event) => event.stopPropagation()}
      >
        <header className="mb-5 flex items-center justify-between gap-4">
          <h2 id="modal-title" className="text-xl font-semibold tracking-tight">{title}</h2>
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
