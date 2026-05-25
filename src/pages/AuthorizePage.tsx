import React, { useEffect, useState } from 'react';
import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { Alert, Button, Spinner, Stack, Text, useStyles2 } from '@grafana/ui';
import { PluginPage, getBackendSrv } from '@grafana/runtime';

export default function AuthorizePage() {
  const styles = useStyles2(getStyles);

  // Extract plugin ID from URL
  const getPluginId = () => {
    const matches = window.location.pathname.match(/\/api\/plugins\/([^\/]+)\//);
    return matches ? matches[1] : 'joshuagrisham-gcxonpremoauth-app';
  };
  const pluginId = getPluginId();

  const [loginStatus, setLoginStatus] =
    useState<'init' | 'working' | 'done' | 'error'>('init');

  const [message, setMessage] = useState('Signing in to Grafana...');
  const [nonce, setNonce] = useState<string | null>(null);
  const [callbackPort, setCallbackPort] = useState<number | null>(null);
  const [tokenName, setTokenName] = useState<string | null>(null);
  const [secondsToLive, setSecondsToLive] = useState<number | null>(null);

  // Parse URL params
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);

    const n = params.get('nonce');
    const p = params.get('callback_port');
    const nameParam = params.get('name');
    const ttl = params.get('secondsToLive');
  
    if (!n || !p) {
      setLoginStatus('error');
      setMessage('missing required parameters');
      return;
    }

    setNonce(n);
    setCallbackPort(Number(p));
    setTokenName(nameParam || `cli-login-${new Date().toISOString()}`);

    if (ttl) {
      const ttlNum = Number(ttl);
      if (!Number.isNaN(ttlNum) && ttlNum > 0) {
        setSecondsToLive(ttlNum);
      }
    }
  }, []);

  // Main login flow
  useEffect(() => {
    if (!nonce || !callbackPort || !tokenName) {
      return;
    }

    const run = async () => {
      try {
        setLoginStatus('working');
        setMessage('Generating token...');

        // Generate the token
        const payload: any = { name: tokenName };
        if (secondsToLive) {
          payload.secondsToLive = secondsToLive;
        }
        const result = await getBackendSrv().post(
          `/api/plugins/${pluginId}/resources/token`,
          payload
        );
        const { key } = result;

        setMessage('Success! Redirecting...');

        // Create a form and POST it to the CLI's local callback endpoint
        // This is more secure than fetch() because the token goes directly
        // from server to server, and the CLI returns a success page
        const form = document.createElement('form');
        form.id = 'cli-callback-form';
        form.method = 'POST';
        form.action = `http://127.0.0.1:${callbackPort}/callback`;

        const nonceInput = document.createElement('input');
        nonceInput.type = 'hidden';
        nonceInput.name = 'nonce';
        nonceInput.value = nonce;
        form.appendChild(nonceInput);

        const tokenInput = document.createElement('input');
        tokenInput.type = 'hidden';
        tokenInput.name = 'token';
        tokenInput.value = key;
        form.appendChild(tokenInput);

        // Submit the form - this will redirect to the CLI's callback
        // The CLI can then respond with a success/error page
        document.body.appendChild(form);
        form.submit();

        // If we get here, the form submission succeeded
        setLoginStatus('done');
        setMessage('Done! <a href="javascript:document.getElementById(\'cli-callback-form\').submit()">Click here</a> if you are not redirected automatically.');
      } catch (err: any) {
        console.error(err);
        setLoginStatus('error');
        setMessage(err.message || 'Sign-in failed');
      }
    };

    run();
  }, [nonce, callbackPort, tokenName, secondsToLive, pluginId]);

  const retry = () => window.location.reload();

  return (
    <PluginPage
      renderTitle={() => <Text variant="h2">Sign in to Grafana</Text>}
      subTitle="Generating user login token..."
    >
      <div className={styles.container}>
        {loginStatus === 'working' && (
          <Stack direction="row" alignItems="center" gap={2}>
            <Spinner />
            <Text>{message}</Text>
          </Stack>
        )}

        {loginStatus === 'done' && (
          <Alert title="Success" severity="success">
            <Stack direction="column" gap={2}>
              <Text>{message}</Text>
              <Button variant="primary" onClick={() => window.close()}>
                Close
              </Button>
            </Stack>
          </Alert>
        )}

        {loginStatus === 'error' && (
          <div className={styles.errorBox}>
            <Alert title="Login failed" severity="error">
              <Text>{message}</Text>
            </Alert>

            <Button
              variant="secondary"
              size="md"
              className={styles.retryButton}
              onClick={retry}
            >
              Retry
            </Button>
          </div>
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
