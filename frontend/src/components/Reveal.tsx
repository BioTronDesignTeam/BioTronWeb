import { useRef, type ElementType, type ReactNode } from 'react';
import { gsap, useGSAP } from '../lib/gsap';
import { useReducedMotion } from '../lib/hooks';

interface RevealProps {
  children: ReactNode;
  as?: ElementType;
  className?: string;
  /** Stagger delay for children (seconds). */
  delay?: number;
  y?: number;
}

/**
 * Fades + lifts an element into view on scroll. Under reduced motion the content
 * is shown immediately with no transform.
 */
export default function Reveal({
  children,
  as: Tag = 'div',
  className,
  delay = 0,
  y = 28,
}: RevealProps) {
  const ref = useRef<HTMLElement>(null);
  const reduced = useReducedMotion();

  useGSAP(
    () => {
      if (reduced || !ref.current) return;
      gsap.from(ref.current, {
        opacity: 0,
        y,
        duration: 0.8,
        delay,
        ease: 'power3.out',
        scrollTrigger: {
          trigger: ref.current,
          start: 'top 85%',
          toggleActions: 'play none none none',
        },
      });
    },
    { scope: ref, dependencies: [reduced] },
  );

  return (
    <Tag ref={ref as never} className={className}>
      {children}
    </Tag>
  );
}
