import { useRef } from 'react';
import { gsap, useGSAP } from '../lib/gsap';
import { useReducedMotion } from '../lib/hooks';

interface CountUpProps {
  value: number;
  suffix?: string;
  /** When true, render the number literally (e.g. a year) without grouping. */
  plain?: boolean;
}

/** Animates a number from 0 to value when scrolled into view. */
export default function CountUp({ value, suffix = '', plain = false }: CountUpProps) {
  const ref = useRef<HTMLSpanElement>(null);
  const reduced = useReducedMotion();

  const format = (n: number) =>
    plain ? Math.round(n).toString() : Math.round(n).toLocaleString('en-US');

  useGSAP(
    () => {
      if (!ref.current) return;
      if (reduced) {
        ref.current.textContent = format(value) + suffix;
        return;
      }
      const obj = { n: 0 };
      gsap.to(obj, {
        n: value,
        duration: 1.6,
        ease: 'power2.out',
        scrollTrigger: { trigger: ref.current, start: 'top 90%' },
        onUpdate: () => {
          if (ref.current) ref.current.textContent = format(obj.n) + suffix;
        },
      });
    },
    { scope: ref, dependencies: [reduced] },
  );

  return <span ref={ref} className="tabular">0{suffix}</span>;
}
