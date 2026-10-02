import { screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { describe, expect, it, vi } from 'vitest';

import SettingsPage from '@/pages/settings/SettingsPage';
import { renderWithProviders } from './test-utils';

// The top bar has its own tests and needs settings the page itself does not.
vi.mock('@/layouts/AppNav', () => ({ default: () => null }));

describe('panel settings', () => {
  // Backup and restore left the traffic page for the panel settings.
  it('opens the backup section from its address', async () => {
    renderWithProviders(
      <MemoryRouter initialEntries={['/settings#backup']}>
        <SettingsPage />
      </MemoryRouter>,
    );

    expect(await screen.findByRole('button', { name: 'Back Up' })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Restore' })).toBeTruthy();
    expect(screen.getByText("Keep this machine's settings")).toBeTruthy();
    // Nothing here is a setting to save, so the save-and-restart bar stays away.
    expect(screen.queryByRole('button', { name: 'Save' })).toBeNull();
  });

  it('keeps the save bar on the other sections', async () => {
    renderWithProviders(
      <MemoryRouter initialEntries={['/settings#general']}>
        <SettingsPage />
      </MemoryRouter>,
    );

    expect(await screen.findByRole('button', { name: 'Save' })).toBeTruthy();
  });
});
