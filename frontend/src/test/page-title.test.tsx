import { MemoryRouter } from 'react-router';
import { expect, test } from 'vitest';

import { usePageTitle } from '@/hooks/usePageTitle';
import { PANEL_NAME } from '@/lib/brand';

import { renderWithProviders } from './test-utils';

function Titled() {
  usePageTitle();
  return null;
}

// A host's own page sits under the hosts tab; without a title it fell back to
// the bare panel name, so the browser tab gave no hint which page it was.
test.each(['/nodes/3', '/nodes/local'])('titles %s after the hosts tab', (path) => {
  renderWithProviders(
    <MemoryRouter initialEntries={[path]}>
      <Titled />
    </MemoryRouter>,
  );
  expect(document.title).toBe(`Hosts · ${PANEL_NAME}`);
});
