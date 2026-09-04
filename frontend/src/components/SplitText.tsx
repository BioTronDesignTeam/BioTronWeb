import { useRef, type ElementType } from 'react';
import { gsap, useGSAP } from '../lib/gsap';
import { useReducedMotion } from '../lib/hooks';

interface SplitTextProps {
  text: string;
  as?: ElementType;
  className?: string;
  /** Animate per 'word' or per 'char'. */
  by?: 'word' | 'char';
  delay?: number;
  trigger?: boolean;
}

/**
 * Reveals text by word/char with a mask-up stagger. Pure-CSS-free dependency:
 * splits into spans manually (no GSAP SplitText plugin needed). Reduced motion
 * renders plain text.
 */
export default function SplitText({
  text,
  as: Tag = 'span',
  className,
  by = 'word',
  delay = 0,
  trigger = false,
}: SplitTextProps) {
  const ref = useRef<HTMLElement>(null);
  const reduced = useReducedMotion();

  const tokens = by === 'word' ? text.split(' ') : Array.from(text);

  useGSAP(
    () => {
      if (reduced || !ref.current) return;
      const parts = ref.current.querySelectorAll<HTMLElement>('[data-split]');
      gsap.from(parts, {
        yPercent: 120,
        opacity: 0,
        duration: 0.9,
        ease: 'power4.out',
        stagger: by === 'char' ? 0.025 : 0.06,
        delay,
        // Promote only while the reveal is running. A permanent will-change
        // holds a compositor layer for every word for the life of the page.
        onStart: () => parts.forEach((part) => (part.style.willChange = 'transform')),
        onComplete: () => parts.forEach((part) => (part.style.willChange = '')),
        ...(trigger
          ? {
              scrollTrigger: { trigger: ref.current, start: 'top 85%' },
            }
          : {}),
      });
    },
    { scope: ref, dependencies: [reduced, text] },
  );

  if (reduced) {
    return <Tag className={className}>{text}</Tag>;
  }

  return (
    <Tag ref={ref as never} className={className} aria-label={text}>
      {tokens.map((tok, i) => (
        <span
          key={i}
          aria-hidden="true"
          style={{ display: 'inline-block', overflow: 'hidden', verticalAlign: 'top' }}
        >
          <span data-split style={{ display: 'inline-block' }}>
            {tok}
            {by === 'word' && i < tokens.length - 1 ? ' ' : ''}
          </span>
        </span>
      ))}
    </Tag>
  );
}
