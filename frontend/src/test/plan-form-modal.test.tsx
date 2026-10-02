import { fireEvent, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import PlanFormModal from '@/pages/plans/PlanFormModal';
import type { PlanSummary } from '@/generated/zod';
import { HttpUtil, Msg } from '@/utils';
import { chooseSelectOption, renderWithProviders } from './test-utils';

const plan: PlanSummary = {
  id: 7,
  name: 'Monthly',
  totalGB: 100 * 1024 ** 3,
  durationDays: 30,
  trafficReset: 'monthly',
  trafficResetDay: 1,
  limitIp: 2,
  remark: '',
  templateId: 0,
  inboundIds: [1, 2],
  memberCount: 3,
  sortIndex: 0,
  createdAt: 0,
  updatedAt: 0,
};

const getStub = vi.mocked(HttpUtil.get);
const setupGet = getStub.getMockImplementation();

afterEach(() => {
  if (setupGet) getStub.mockImplementation(setupGet);
});

const TEMPLATES = [
  {
    id: 3,
    name: 'alpha_v3',
    isDefault: true,
    kind: 'yaml',
    size: 10,
    planCount: 2,
    updatedAt: 0,
  },
  {
    id: 5,
    name: 'beta_v3',
    isDefault: false,
    kind: 'yaml',
    size: 10,
    planCount: 0,
    updatedAt: 0,
  },
];

function save() {
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));
}

describe('PlanFormModal', () => {
  // Ticked by default, every edit re-stamped the members' limits, even one that only added a server.
  it('saves an edit without re-applying limits unless the box is ticked', async () => {
    const onConfirm = vi.fn();
    renderWithProviders(
      <PlanFormModal open plan={plan} onClose={() => {}} onConfirm={onConfirm} />,
    );
    const reapply = screen.getByRole('checkbox', {
      name: 'Also re-apply quota, IP limit and reset schedule to the 3 user(s) on this plan',
    }) as HTMLInputElement;
    expect(reapply.checked).toBe(false);
    expect(
      screen.getByText(
        "Nodes you add or remove always reach this plan's users, with or without this option.",
      ),
    ).toBeTruthy();

    save();
    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
    expect(onConfirm.mock.calls[0][1]).toBe(false);

    fireEvent.click(reapply);
    save();
    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(2));
    expect(onConfirm.mock.calls[1][1]).toBe(true);
  });

  // The rules box became a template picker; 0 stays on the default template.
  it('names the default template and saves the one picked', async () => {
    getStub.mockImplementation(async (url: string) =>
      url === '/panel/api/ruleTemplates/list'
        ? new Msg(true, '', TEMPLATES)
        : new Msg(true, '', []),
    );
    const onConfirm = vi.fn();
    renderWithProviders(
      <PlanFormModal open plan={plan} onClose={() => {}} onConfirm={onConfirm} />,
    );

    await screen.findByText('Default (alpha_v3)');
    chooseSelectOption(screen.getByLabelText('Rule template').id, 'beta_v3');
    save();

    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
    expect(onConfirm.mock.calls[0][0]).toMatchObject({ templateId: 5 });
  });
});
