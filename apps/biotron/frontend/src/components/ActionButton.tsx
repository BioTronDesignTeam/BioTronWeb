import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';

interface ActionButtonProps {
  children: ReactNode;
  to?: string;
  href?: string;
  onClick?: () => void;
  variant?: 'primary' | 'ghost';
  className?: string;
  accent?: string;
}

/** Shared action with restrained hover and keyboard-focus feedback. */
export default function ActionButton({
  children,
  to,
  href,
  onClick,
  variant = 'primary',
  className = '',
  accent,
}: ActionButtonProps) {
  const cls = `actionbtn actionbtn--${variant} ${className}`;
  const style = accent ? ({ ['--btn-accent' as string]: accent }) : undefined;
  const inner = <span className="actionbtn__inner">{children}</span>;
  const shared = { className: cls, style, onClick };

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
