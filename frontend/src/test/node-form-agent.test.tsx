import { describe, it, expect, vi } from 'vitest';
import { fireEvent, screen, waitFor } from '@testing-library/react';

import NodeFormModal from '@/pages/nodes/NodeFormModal';
import NodeList from '@/pages/nodes/NodeList';
import type { NodeRecord } from '@/schemas/node';
import { MemoryRouter } from 'react-router';

import { renderWithProviders } from './test-utils';

function renderForm(mode: 'add' | 'edit', node: NodeRecord | null) {
  const props = {
    testConnection: vi.fn(),
    fetchFingerprint: vi.fn(),
    fetchInbounds: vi.fn(),
    save: vi.fn().mockResolvedValue({ success: true, msg: '', obj: { id: 7 } }),
    mintAgentSecret: vi
      .fn()
      .mockResolvedValue({ success: true, msg: '', obj: { secret: 'S3CR3T' } }),
    onOpenChange: vi.fn(),
  };
  renderWithProviders(<NodeFormModal open mode={mode} node={node} {...props} />);
  return props;
}

function typeInto(label: string, value: string) {
  fireEvent.change(screen.getByLabelText(label), { target: { value } });
}

function submit() {
  const ok = document.querySelector('.ant-modal-footer .ant-btn-primary');
  if (!ok) throw new Error('save button not found');
  fireEvent.click(ok);
}

describe('NodeFormModal agent nodes', () => {
  // An agent dials the panel only after it is installed with the secret, so a
  // reachability probe before saving would refuse every new agent.
  it('saves an agent without probing it and shows its secret once', async () => {
    const props = renderForm('add', null);
    fireEvent.click(screen.getByText('Agent'));
    typeInto('Name', 'lazycat');
    typeInto('Public address', '216.236.63.53');
    submit();

    await waitFor(() => expect(props.mintAgentSecret).toHaveBeenCalledWith(7));
    expect(props.testConnection).not.toHaveBeenCalled();
    expect(props.save).toHaveBeenCalledWith({
      id: 0,
      kind: 'agent',
      name: 'lazycat',
      remark: '',
      address: '216.236.63.53',
      enable: true,
    });
    await waitFor(() => {
      const values = Array.from(document.querySelectorAll('input')).map((el) => el.value);
      expect(values.some((v) => v.includes('install.sh') && v.includes('S3CR3T'))).toBe(true);
    });
  });

  // Minting replaces the secret and drops the connected agent, so editing a
  // remark must not mint; turning a panel node into an agent must.
  it('mints a secret only when a node becomes an agent', async () => {
    const agent = renderForm('edit', {
      id: 3,
      name: 'frontier',
      kind: 'agent',
      address: '66.132.239.17',
      enable: true,
    });
    typeInto('Remark', 'US west');
    submit();
    await waitFor(() => expect(agent.save).toHaveBeenCalled());
    expect(agent.mintAgentSecret).not.toHaveBeenCalled();
  });

  it('mints a secret when a panel node is converted', async () => {
    const converted = renderForm('edit', {
      id: 5,
      name: 'lazycat',
      kind: 'panel',
      scheme: 'http',
      address: '127.0.0.1',
      port: 22605,
      hasApiToken: true,
      enable: true,
    });
    fireEvent.click(screen.getByText('Agent'));
    typeInto('Public address', '216.236.63.53');
    submit();
    await waitFor(() => expect(converted.mintAgentSecret).toHaveBeenCalledWith(5));
    expect(converted.testConnection).not.toHaveBeenCalled();
  });
});

describe('NodeList agent nodes', () => {
  it('offers no panel update for an agent', () => {
    const noop = () => {};
    renderWithProviders(
      <MemoryRouter>
        <NodeList
          nodes={[
            {
              id: 1,
              name: 'panel',
              kind: 'panel',
              enable: true,
              status: 'online',
              address: 'a.example.com',
              port: 2053,
              scheme: 'https',
            },
            {
              id: 2,
              name: 'agent',
              kind: 'agent',
              enable: true,
              status: 'online',
              address: '203.0.113.9',
            },
          ]}
          isMobile={false}
          selectedIds={[]}
          onSelectionChange={noop}
          onAdd={noop}
          onMtls={noop}
          onEdit={noop}
          onDelete={noop}
          onProbe={noop}
          onToggleEnable={noop}
          onUpdateNode={noop}
          onUpdateSelected={noop}
        />
      </MemoryRouter>,
    );
    const updateButtons = document.querySelectorAll('button[aria-label="Update Panel"]');
    expect(updateButtons).toHaveLength(1);
    expect(screen.getByText('203.0.113.9').closest('a')).toBeNull();
  });
});
