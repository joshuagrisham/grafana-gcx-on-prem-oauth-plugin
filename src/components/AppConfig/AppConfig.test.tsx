import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import { PluginType, type AppPluginMeta } from '@grafana/data';
import AppConfig, { AppConfigProps } from './AppConfig';

const fetchMock = jest.fn();
const reloadMock = jest.fn();

jest.mock('@grafana/runtime', () => ({
  ...jest.requireActual('@grafana/runtime'),
  getBackendSrv: () => ({
    fetch: (...args: unknown[]) => fetchMock(...args),
  }),
}));

const baseMeta = {
  id: 'joshuagrisham-gcxonpremoauth-app',
  name: 'gcx On-Prem OAuth',
  type: PluginType.app,
  enabled: true,
  pinned: false,
  jsonData: {},
  secureJsonFields: {},
} as unknown as AppPluginMeta<{ token?: string }>;

const makeProps = (overrides: Partial<AppPluginMeta<{ token?: string }>> = {}): AppConfigProps =>
  ({
    plugin: { meta: { ...baseMeta, ...overrides } },
    query: {},
  }) as unknown as AppConfigProps;

beforeEach(() => {
  fetchMock.mockReset();
  // Resolve fetch() into an Observable-like value that lastValueFrom() can read.
  fetchMock.mockReturnValue({
    subscribe: (cb: { next?: (v: unknown) => void; complete?: () => void }) => {
      cb.next?.({ data: {} });
      cb.complete?.();
      return { unsubscribe: () => {} };
    },
    // rxjs lastValueFrom checks for Symbol.observable; this minimal shape
    // is sufficient for our tests since onSubmit doesn't await the response.
    [Symbol.observable ?? '@@observable']() {
      return this;
    },
  });
  reloadMock.mockReset();
  // jsdom's window.location.reload is not assignable; stub via defineProperty.
  Object.defineProperty(window, 'location', {
    value: { ...window.location, reload: reloadMock },
    writable: true,
  });
});

describe('Components/AppConfig', () => {
  test('renders the Organization Service Account Token field and submit button', () => {
    render(<AppConfig {...makeProps()} />);
    expect(screen.getByRole('group', { name: /organization service account token/i })).toBeInTheDocument();
    expect(screen.getByPlaceholderText('glsa_...')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /save token/i })).toBeInTheDocument();
  });

  test('disables the submit button when a token is already configured', () => {
    render(<AppConfig {...makeProps({ secureJsonFields: { token: true } })} />);
    expect(screen.getByRole('button', { name: /save token/i })).toBeDisabled();
  });

  test('exposes a reset control which re-enables editing the token', () => {
    render(<AppConfig {...makeProps({ secureJsonFields: { token: true } })} />);
    fireEvent.click(screen.getByRole('button', { name: /reset/i }));
    expect(screen.getByRole('button', { name: /save token/i })).toBeEnabled();
  });

  test('submitting an edited token POSTs to the plugin settings API', () => {
    render(<AppConfig {...makeProps()} />);
    const input = screen.getByPlaceholderText('glsa_...');
    fireEvent.change(input, { target: { value: 'glsa_test_value' } });
    fireEvent.click(screen.getByRole('button', { name: /save token/i }));

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const callArg = fetchMock.mock.calls[0][0];
    expect(callArg.method).toBe('POST');
    expect(callArg.url).toBe(`/api/plugins/${baseMeta.id}/settings`);
    expect(callArg.data.secureJsonData).toEqual({ token: 'glsa_test_value' });
  });
});
