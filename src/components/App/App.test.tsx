import React from 'react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { AppRootProps, PluginType } from '@grafana/data';
import { render, screen, waitFor } from '@testing-library/react';
import App from './App';
import { ROUTES } from '../../constants';

// Mock the lazy-loaded pages so we don't have to deal with @grafana/runtime
// network calls during unit tests. The App component lazy-imports them, so
// the mocks must be set up before the component is rendered.
jest.mock('../../pages/MenuPage', () => ({
  __esModule: true,
  default: () => <div>menu-page</div>,
}));
jest.mock('../../pages/AuthorizePage', () => ({
  __esModule: true,
  default: () => <div>authorize-page</div>,
}));
jest.mock('../../pages/TokensPage', () => ({
  __esModule: true,
  default: () => <div>tokens-page</div>,
}));

const baseProps = {
  basename: 'a/test-app',
  meta: {
    id: 'test-app',
    name: 'Test App',
    type: PluginType.app,
    enabled: true,
    jsonData: {},
  },
  query: {},
  path: '',
  onNavChanged: jest.fn(),
} as unknown as AppRootProps;

const renderAt = (path: string) =>
  render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/*" element={<App {...baseProps} />} />
      </Routes>
    </MemoryRouter>
  );

describe('Components/App routing', () => {
  test('renders the menu page on the menu route', async () => {
    renderAt(`/${ROUTES.Menu}`);
    await waitFor(() => expect(screen.getByText('menu-page')).toBeInTheDocument());
  });

  test('renders the tokens page on the tokens route', async () => {
    renderAt(`/${ROUTES.Tokens}`);
    await waitFor(() => expect(screen.getByText('tokens-page')).toBeInTheDocument());
  });

  test('renders the authorize page on the authorize route', async () => {
    renderAt(`/${ROUTES.Authorize}`);
    await waitFor(() => expect(screen.getByText('authorize-page')).toBeInTheDocument());
  });

  test('falls back to the menu page on an unknown route', async () => {
    renderAt('/something-unknown');
    await waitFor(() => expect(screen.getByText('menu-page')).toBeInTheDocument());
  });
});
