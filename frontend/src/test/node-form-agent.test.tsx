import { describe, it, expect, vi } from 'vitest';
import { fireEvent, screen, waitFor } from '@testing-library/react';

import NodeFormModal from '@/pages/nodes/NodeFormModal';
import NodeList from '@/pages/nodes/NodeList';
import type { NodeRecord } from '@/schemas/node';
import { MemoryRouter } from 'react-router';

import { chooseSelectOption, listSelectOptions, renderWithProviders } from './test-utils';

function renderForm(mode: 'add' | 'edit', node: NodeRecord | null, withProbe = false) {
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
  renderWithProviders(
    <NodeFormModal
      open
      mode={mode}
      node={node}
      probeServers={
        withProbe
          ? [
              { id: 'free-lite', name: 'Example unlinked host', linked: false },
              { id: 'used-lite', name: 'Already managed host', linked: true },
            ]
          : []
      }
      {...props}
    />,
  );
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
  it('adds an unlinked probe server without requiring its address', async () => {
    const props = renderForm('add', null, true);
    const field = screen.getByLabelText('From probe').id;
    expect(listSelectOptions(field)).toEqual(['Example unlinked host']);
    chooseSelectOption(field, 'Example unlinked host');
    expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe('Example unlinked host');
    expect((screen.getByLabelText('Public address') as HTMLInputElement).value).toBe('');
    submit();
    await waitFor(() => expect(props.mintAgentSecret).toHaveBeenCalledWith(7));
    expect(props.save).toHaveBeenCalledWith(
      expect.objectContaining({ kind: 'agent', probeServerId: 'free-lite', address: '' }),
    );
    expect(props.testConnection).not.toHaveBeenCalled();
  });
  // An agent dials the panel only after it is installed with the secret, so a
  // reachability probe before saving would refuse every new agent.
  it('saves an agent without probing it and shows its secret once', async () => {
    const props = renderForm('add', null);
    fireEvent.click(screen.getByText('Agent'));
    typeInto('Name', 'edge-hk');
    typeInto('Public address', '203.0.113.53');
    submit();

    await waitFor(() => expect(props.mintAgentSecret).toHaveBeenCalledWith(7));
    expect(props.testConnection).not.toHaveBeenCalled();
    expect(props.save).toHaveBeenCalledWith({
      id: 0,
      kind: 'agent',
      name: 'edge-hk',
      remark: '',
      address: '203.0.113.53',
      enable: true,
      trafficMultiplier: 1,
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
      name: 'edge-us',
      kind: 'agent',
      address: '203.0.113.17',
      enable: true,
    });
    typeInto('Remark', 'US west');
    submit();
    await waitFor(() => expect(agent.save).toHaveBeenCalled());
    expect(agent.mintAgentSecret).not.toHaveBeenCalled();
  });

  // Users' bytes on this host count toward their quotas at this rate.
  it("saves an agent's traffic multiplier", async () => {
    const props = renderForm('edit', {
      id: 3,
      name: 'edge-us',
      kind: 'agent',
      address: '203.0.113.17',
      enable: true,
      trafficMultiplier: 0.1,
    });
    const field = screen.getByLabelText('Traffic multiplier') as HTMLInputElement;
    expect(field.value).toBe('0.1');
    typeInto('Traffic multiplier', '0.5');
    submit();
    await waitFor(() =>
      expect(props.save).toHaveBeenCalledWith(expect.objectContaining({ trafficMultiplier: 0.5 })),
    );
  });

  // An agent added from the probe is stored with port 0, a panel-only field the
  // dialog hides; it must not block a save with "expected number to be >=1".
  it('saves an agent stored without a port', async () => {
    const props = renderForm('edit', {
      id: 21,
      name: 'edge-de',
      kind: 'agent',
      address: '203.0.113.21',
      port: 0,
      scheme: '',
      tlsVerifyMode: 'verify',
      enable: true,
      trafficMultiplier: 1,
    });
    typeInto('Traffic multiplier', '0.2');
    submit();
    await waitFor(() =>
      expect(props.save).toHaveBeenCalledWith(
        expect.objectContaining({ kind: 'agent', trafficMultiplier: 0.2 }),
      ),
    );
  });

  // A child panel limits users on the bytes it counts itself, so it has no multiplier.
  it('offers no traffic multiplier for a 3x-ui child panel', () => {
    renderForm('edit', {
      id: 5,
      name: 'edge-hk',
      kind: 'panel',
      scheme: 'https',
      address: 'panel.example.com',
      port: 2053,
      hasApiToken: true,
      enable: true,
    });
    expect(screen.queryByLabelText('Traffic multiplier')).toBeNull();
  });

  it('mints a secret when a panel node is converted', async () => {
    const converted = renderForm('edit', {
      id: 5,
      name: 'edge-hk',
      kind: 'panel',
      scheme: 'http',
      address: '127.0.0.1',
      port: 24005,
      hasApiToken: true,
      enable: true,
    });
    fireEvent.click(screen.getByText('Agent'));
    typeInto('Public address', '203.0.113.53');
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
          showAddress={false}
          onShowAddressChange={noop}
          onEdit={noop}
          onDelete={noop}
          onProbe={noop}
          onToggleEnable={noop}
          onUpdateNode={noop}
        />
      </MemoryRouter>,
    );
    const updateButtons = document.querySelectorAll('button[aria-label="Update Panel"]');
    expect(updateButtons).toHaveLength(1);
    expect(screen.getByText('203.0.113.9').closest('a')).toBeNull();
  });
});
