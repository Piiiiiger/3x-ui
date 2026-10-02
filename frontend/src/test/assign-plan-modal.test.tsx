import { fireEvent, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import AssignPlanModal from '@/pages/plans/AssignPlanModal';
import type { PlanSummary } from '@/generated/zod';
import { HttpUtil, Msg } from '@/utils';
import { chooseSelectOption, renderWithProviders } from './test-utils';

const PLANS: PlanSummary[] = [
  {
    id: 2,
    name: 'Monthly',
    limitIp: 2,
    remark: '',
    templateId: 0,
    inboundIds: [1],
    memberCount: 0,
    sortIndex: 0,
    createdAt: 0,
    updatedAt: 0,
  },
];

const postStub = vi.mocked(HttpUtil.post);
const setupPost = postStub.getMockImplementation();

afterEach(() => {
  if (setupPost) postStub.mockImplementation(setupPost);
});

describe('AssignPlanModal', () => {
  // A plan no longer sets quota or validity, so assigning asks only which plan.
  it('puts the users on the plan without touching their own limits', async () => {
    postStub.mockResolvedValue(new Msg(true, '', { affected: 1 }));
    renderWithProviders(
      <AssignPlanModal open plans={PLANS} emails={['alice']} onClose={() => {}} />,
    );
    expect(screen.queryAllByRole('radio')).toHaveLength(0);
    expect(screen.queryAllByRole('checkbox')).toHaveLength(0);

    chooseSelectOption(screen.getByLabelText('Plans').id, 'Monthly');
    fireEvent.click(screen.getByRole('button', { name: 'Confirm' }));

    await waitFor(() =>
      expect(postStub).toHaveBeenCalledWith(
        '/panel/api/plans/assign',
        { emails: ['alice'], planId: 2 },
        expect.anything(),
      ),
    );
  });
});
