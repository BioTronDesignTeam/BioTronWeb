import type { HTMLAttributes } from 'react';
import handMark from './assets/biotron-hand.png';
import blueWordmark from './assets/biotron-wordmark-blue.png';
import whiteWordmark from './assets/biotron-wordmark-white.webp';

export type BrandProps = HTMLAttributes<HTMLSpanElement> & {
  compact?: boolean;
  label?: string;
};

export function Brand({ className = '', compact = false, label = 'BioTron', ...props }: BrandProps) {
  return (
    <span className={`biotron-brand ${compact ? 'biotron-brand--compact' : ''} ${className}`.trim()} {...props}>
      {compact ? (
        <img className="biotron-brand__hand" src={handMark} alt={label} />
      ) : (
        <>
          <img className="biotron-brand__wordmark biotron-brand__wordmark--light" src={blueWordmark} alt={label} />
          <img className="biotron-brand__wordmark biotron-brand__wordmark--dark" src={whiteWordmark} alt={label} />
        </>
      )}
    </span>
  );
}
