import type { ButtonHTMLAttributes } from 'react';

export type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  tone?: 'primary' | 'neutral';
};

export function Button({ className = '', tone = 'primary', ...props }: ButtonProps) {
  return (
    <button
      className={`biotron-button biotron-button--${tone} ${className}`.trim()}
      {...props}
    />
  );
}
