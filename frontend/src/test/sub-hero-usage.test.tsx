import { screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import SubHero from '@/pages/sub/SubHero';
import { renderWithProviders } from './test-utils';

const GIB = 1024 ** 3;

function renderHero(usedByte: number, totalByte: number, used: string, total: string) {
  renderWithProviders(
    <SubHero
      status={totalByte > 0 ? 'active' : 'unlimited'}
      daysLeft={12}
      usedByte={usedByte}
      totalByte={totalByte}
      expireMs={0}
      lastOnlineMs={0}
      download="20.00GB"
      upload="5.00GB"
      used={used}
      total={total}
      remained="75.00GB"
      datepicker="gregorian"
      lang="en-US"
    />,
  );
}

// The portal and the /x/<subId> page share this hero; it showed the usage only as a ring.
describe("the usage bar on a person's page", () => {
  it('fills the share of the quota used', () => {
    renderHero(25 * GIB, 100 * GIB, '25.00GB', '100.00GB');
    const bar = screen.getByRole('progressbar', { name: 'Usage 25.0 %' });
    expect(bar.getAttribute('aria-valuetext')).toBe('25.00GB / 100.00GB');
    expect(screen.getAllByRole('progressbar')).toHaveLength(1);
    expect(screen.getByRole('status').textContent).toBe('Active');
  });

  it('runs full without a quota and says there is none', () => {
    renderHero(3 * GIB, 0, '3.00GB', '∞');
    const bar = screen.getByRole('progressbar', { name: 'Usage 3.00GB / Unlimited' });
    expect(bar.hasAttribute('aria-valuenow')).toBe(false);
  });
});
