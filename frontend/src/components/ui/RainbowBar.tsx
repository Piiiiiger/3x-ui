import type { CSSProperties } from 'react';

import './RainbowBar.css';

interface RainbowBarProps {
  // null when there is no limit, which 妙妙屋X draws as a full bar.
  percent: number | null;
  label: string;
  valueText: string;
  className?: string;
}

export default function RainbowBar({ percent, label, valueText, className }: RainbowBarProps) {
  const fill = percent === null ? 100 : Math.min(100, percent);
  const value = percent === null ? valueText : `${percent.toFixed(1)} %`;
  return (
    <div
      className={className ? `rainbow-bar ${className}` : 'rainbow-bar'}
      role="progressbar"
      aria-label={`${label} ${value}`}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={percent === null ? undefined : fill}
      aria-valuetext={valueText}
      style={{ '--rainbow-fill': `${fill}%` } as CSSProperties}
    >
      <span className="rainbow-bar-fill" />
    </div>
  );
}
