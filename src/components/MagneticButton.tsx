import { useRef, type ReactNode, type MouseEvent } from 'react';
import { Link } from 'react-router-dom';
import { gsap } from '../lib/gsap';
import { useHasFinePointer, useReducedMotion } from '../lib/hooks';

interface MagneticButtonProps {
  children: ReactNode;
  to?: string;
  href?: string;
  onClick?: () => void;
  variant?: 'primary' | 'ghost';
  className?: string;
  accent?: string;
}

/**
 * Button/link with a magnetic pull toward the cursor on fine pointers.
 * Degrades to a plain styled control on touch / reduced motion.
 */
export default function MagneticButton({
  children,
  to,
  href,
  onClick,
  variant = 'primary',
  className = '',
  accent,
}: MagneticButtonProps) {
  const ref = useRef<HTMLSpanElement>(null);
  const fine = useHasFinePointer();
  const reduced = useReducedMotion();
  const magnetic = fine && !reduced;

  const onMove = (e: MouseEvent) => {
    if (!magnetic || !ref.current) return;
    const r = ref.current.getBoundingClientRect();
    const x = e.clientX - (r.left + r.width / 2);
    const y = e.clientY - (r.top + r.height / 2);
    gsap.to(ref.current, { x: x * 0.3, y: y * 0.4, duration: 0.4, ease: 'power3.out' });
  };
  const onLeave = () => {
    if (ref.current) gsap.to(ref.current, { x: 0, y: 0, duration: 0.5, ease: 'elastic.out(1,0.4)' });
  };

  const cls = `magbtn magbtn--${variant} ${className}`;
  const style = accent ? ({ ['--btn-accent' as string]: accent }) : undefined;
  const inner = (
    <span ref={ref} className="magbtn__inner">
      {children}
    </span>
  );

  const shared = { className: cls, style, onMouseMove: onMove, onMouseLeave: onLeave, onClick };

  if (to) {
    return (
      <Link to={to} {...shared}>
        {inner}
      </Link>
    );
  }
  if (href) {
    return (
      <a href={href} target="_blank" rel="noreferrer" {...shared}>
        {inner}
      </a>
    );
  }
  return (
    <button type="button" {...shared}>
      {inner}
    </button>
  );
}
