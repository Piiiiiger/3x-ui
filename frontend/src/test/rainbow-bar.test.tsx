import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { RainbowBar } from '@/components/ui';

const fillOf = (bar: HTMLElement) => bar.style.getPropertyValue('--rainbow-fill');

describe('RainbowBar', () => {
  it('fills the share used and reads it out with the figures', () => {
    render(<RainbowBar percent={25} label="Traffic quota" valueText="25.00 GB / 100.00 GB" />);
    const bar = screen.getByRole('progressbar', { name: 'Traffic quota 25.0 %' });
    expect(bar.getAttribute('aria-valuenow')).toBe('25');
    expect(bar.getAttribute('aria-valuetext')).toBe('25.00 GB / 100.00 GB');
    expect(fillOf(bar)).toBe('25%');
  });

  // Over the quota the bar stops at full, while the label still says by how much.
  it('keeps an overrun inside the bar', () => {
    render(<RainbowBar percent={140} label="Traffic quota" valueText="140.00 GB / 100.00 GB" />);
    const bar = screen.getByRole('progressbar', { name: 'Traffic quota 140.0 %' });
    expect(bar.getAttribute('aria-valuenow')).toBe('100');
    expect(fillOf(bar)).toBe('100%');
  });

  it('runs full without a limit and claims no share', () => {
    render(<RainbowBar percent={null} label="Traffic quota" valueText="3.00 GB / Unlimited" />);
    const bar = screen.getByRole('progressbar', { name: 'Traffic quota 3.00 GB / Unlimited' });
    expect(bar.hasAttribute('aria-valuenow')).toBe(false);
    expect(fillOf(bar)).toBe('100%');
  });
});
