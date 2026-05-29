import React, { useEffect, useMemo, useRef, useState } from 'react';
import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { Alert, Button, LoadingPlaceholder, Stack, Text, useStyles2 } from '@grafana/ui';
import { PluginPage, getBackendSrv } from '@grafana/runtime';
import { lastValueFrom } from 'rxjs';
import { PLUGIN_RESOURCES_URL } from '../constants';

/**
 * "Authorize" page.
 * Provides an OAuth-like /authorize endpoint that handles token generation
 * and callback to the CLI's loopback service.
 */

type Status = 'init' | 'working' | 'submitting' | 'done' | 'error';

const MIN_PORT = 1;
const MAX_PORT = 65535;
const FORM_ID = 'cli-callback-form';

interface AuthorizeParams {
  state: string;
  callbackPort: number;
  tokenName: string;
  secondsToLive?: number;
}

const parseParams = (): { params: AuthorizeParams | null; error: string | null } => {
  const search = new URLSearchParams(window.location.search);
  const state = search.get('state');
  const port = search.get('callback_port');
  const nameParam = search.get('name');
  const ttlParam = search.get('secondsToLive');

  if (!state || !port) {
    return { params: null, error: 'Missing required parameters (state, callback_port).' };
  }
  const portNum = Number(port);
  if (!Number.isInteger(portNum) || portNum < MIN_PORT || portNum > MAX_PORT) {
    return { params: null, error: `callback_port must be an integer between ${MIN_PORT} and ${MAX_PORT}.` };
  }
  let secondsToLive: number | undefined;
  if (ttlParam) {
    const ttl = Number(ttlParam);
    if (!Number.isInteger(ttl) || ttl <= 0) {
      return { params: null, error: 'secondsToLive must be a positive integer.' };
    }
    secondsToLive = ttl;
  }
  return {
    params: {
      state,
      callbackPort: portNum,
      tokenName: nameParam || `cli-login-${new Date().toISOString()}`,
      secondsToLive,
    },
    error: null,
  };
};

export default function AuthorizePage() {
  const styles = useStyles2(getStyles);
  const { params, error: parseError } = useMemo(() => parseParams(), []);

  const [status, setStatus] = useState<Status>(parseError ? 'error' : 'init');
  const [message, setMessage] = useState<string>(parseError ?? '');
  const [tokenKey, setTokenKey] = useState<string | null>(null);
  const formRef = useRef<HTMLFormElement | null>(null);
  const hasStartedRef = useRef(false);

  useEffect(() => {
    if (!params || hasStartedRef.current || status !== 'init') {
      return;
    }
    hasStartedRef.current = true;

    const run = async () => {
      try {
        setStatus('working');
        setMessage('Generating token...');

        const payload: Record<string, unknown> = { name: params.tokenName };
        if (params.secondsToLive) {
          payload.secondsToLive = params.secondsToLive;
        }
        const response = await getBackendSrv().fetch<{ key: string; warnings?: string[] }>({
          url: `${PLUGIN_RESOURCES_URL}/token`,
          method: 'POST',
          data: payload,
        });
        const result = await lastValueFrom(response);
        const key = result.data?.key;
        if (!key) {
          throw new Error('Backend did not return a token key.');
        }
        setTokenKey(key);
        setMessage('Sending token back to gcx...');
        setStatus('submitting');
      } catch (err: unknown) {
        console.error(err);
        setStatus('error');
        setMessage(err instanceof Error ? err.message : 'Sign-in failed');
      }
    };
    void run();
  }, [params, status]);

  // Auto-submit the form once the token is ready. The token is delivered
  // to the CLI by a regular form submission so the CLI can render its own
  // success/error page.
  useEffect(() => {
    if (status === 'submitting' && formRef.current) {
      formRef.current.submit();
      setStatus('done');
    }
  }, [status]);

  const retry = () => window.location.reload();
  const manualSubmit = () => formRef.current?.submit();

  return (
    <PluginPage
      renderTitle={() => <Text variant="h2">Sign in to gcx</Text>}
      subTitle="Generating a CLI login token..."
    >
      <div className={styles.container}>
        {(status === 'working' || status === 'submitting') && (
          <Stack direction="row" alignItems="center" gap={2}>
            <LoadingPlaceholder text={message} />
          </Stack>
        )}

        {status === 'done' && params && (
          <Alert title="Success" severity="success">
            <Stack direction="column" gap={2}>
              <Text>
                A token was generated and posted to the gcx CLI on{' '}
                <code>{`http://127.0.0.1:${params.callbackPort}/callback`}</code>. You can close this window.
              </Text>
              <Text variant="bodySmall" color="secondary">
                Not redirected automatically? Click below to resend the token to the CLI.
              </Text>
              <Stack direction="row" gap={2}>
                <Button variant="primary" onClick={manualSubmit}>
                  Resend to CLI
                </Button>
                <Button variant="secondary" onClick={() => window.close()}>
                  Close window
                </Button>
              </Stack>
            </Stack>
          </Alert>
        )}

        {status === 'error' && (
          <div className={styles.errorBox}>
            <Alert title="Sign-in failed" severity="error">
              <Text>{message}</Text>
            </Alert>
            <Button variant="secondary" size="md" className={styles.retryButton} onClick={retry}>
              Retry
            </Button>
          </div>
        )}

        {/* Hidden POST form posted to the CLI's loopback callback. */}
        {tokenKey && params && (
          <form
            ref={formRef}
            id={FORM_ID}
            method="POST"
            action={`http://127.0.0.1:${params.callbackPort}/callback`}
            style={{ display: 'none' }}
          >
            <input type="hidden" name="state" value={params.state} />
            <input type="hidden" name="token" value={tokenKey} />
          </form>
        )}
      </div>
    </PluginPage>
  );
}

const getStyles = (theme: GrafanaTheme2) => ({
  container: css`
    margin-top: ${theme.spacing(4)};
    max-width: 600px;
  `,
  errorBox: css`
    display: flex;
    flex-direction: column;
    gap: 16px;
    max-width: 400px;
  `,
  retryButton: css`
    width: fit-content;
  `,
});
