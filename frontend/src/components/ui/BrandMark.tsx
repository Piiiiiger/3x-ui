import { PANEL_NAME } from '@/lib/brand';
import './BrandMark.css';

// The panel's name with its first letter animated, as 妙妙屋X animates its X.
export default function BrandMark({ className }: { className?: string }) {
  const initial = PANEL_NAME.slice(0, 1);
  return (
    <span className={className ? `brand-mark ${className}` : 'brand-mark'}>
      <span className="brand-mark-initial">
        <span className="brand-mark-letter">{initial}</span>
        <span className="brand-mark-glow" aria-hidden="true">
          {initial}
        </span>
        <span className="brand-mark-sparkles" aria-hidden="true" />
      </span>
      <span className="brand-mark-rest">{PANEL_NAME.slice(1)}</span>
    </span>
  );
}
