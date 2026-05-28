import React, { useCallback, useEffect, useState } from 'react';
import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { Alert, Button, LoadingPlaceholder, useStyles2 } from '@grafana/ui';
import { PluginPage, getBackendSrv } from '@grafana/runtime';
import { lastValueFrom } from 'rxjs';
import { CreateTokenModal } from './components/CreateTokenModal';
import { ServiceAccountTokensTable } from './components/ServiceAccountTokensTable';
import { PLUGIN_RESOURCES_URL } from '../constants';

// Display name used by CreateTokenModal when seeding a default token name.
// Showing "grafana-gui" makes it obvious in the token list that the token
// was minted from the Grafana UI rather than from the gcx CLI.
const TOKEN_NAME_PREFIX = 'grafana-gui';

export type Token = {
  id: number;
  name: string;
  key?: string;
  created?: string;
  expiration?: string;
  hasExpired: boolean;
  lastUsedAt?: string;
};

// Type alias so Grafana's vendored components work with our Token type.
export type ApiKey = Token & {
  isRevoked?: boolean;
  secondsUntilExpiration?: number;
};

type CreateResponse = Token & { warnings?: string[] };

const enrichedToken = (token: Token): ApiKey => ({
  ...token,
  isRevoked: false,
  secondsUntilExpiration: token.expiration
    ? Math.max(0, Math.floor((new Date(token.expiration).getTime() - Date.now()) / 1000))
    : undefined,
});

const TokensPage = () => {
  const s = useStyles2(getStyles);

  const [tokens, setTokens] = useState<ApiKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [newToken, setNewToken] = useState<string>('');
  const [noExpirationInfo, setNoExpirationInfo] = useState(false);

  const fetchTokens = useCallback(async () => {
    try {
      const response = await getBackendSrv().fetch<Token[]>({
        method: 'GET',
        url: `${PLUGIN_RESOURCES_URL}/tokens`,
      });
      const result = await lastValueFrom(response);
      setTokens((result.data ?? []).map(enrichedToken));
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch tokens');
    }
  }, []);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      await fetchTokens();
      if (!cancelled) {
        setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [fetchTokens]);

  const handleCreateToken = async (data: { name: string; secondsToLive?: number }) => {
    try {
      const response = await getBackendSrv().fetch<CreateResponse>({
        method: 'POST',
        url: `${PLUGIN_RESOURCES_URL}/token`,
        data: { name: data.name, secondsToLive: data.secondsToLive },
      });
      const result = await lastValueFrom(response);
      const created = result.data;
      setNewToken(created?.key ?? '');

      // If the user selected "No expiration" in the modal, inform them that the
      // token was created with the plugin's default max TTL instead.
      if (!data.secondsToLive) {
        setNoExpirationInfo(true);
      }

      await fetchTokens();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create token');
    }
  };

  const handleDeleteToken = async (token: ApiKey) => {
    try {
      const response = await getBackendSrv().fetch({
        method: 'DELETE',
        url: `${PLUGIN_RESOURCES_URL}/tokens/${token.id}`,
      });
      await lastValueFrom(response);
      await fetchTokens();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete token');
    }
  };

  if (loading) {
    return (
      <PluginPage>
        <LoadingPlaceholder text="Loading tokens..." />
      </PluginPage>
    );
  }

  return (
    <PluginPage>
      <div>
        {error && (
          <Alert title="Error" severity="error" onRemove={() => setError(null)}>
            {error}
          </Alert>
        )}

        {noExpirationInfo && (
          <Alert
            title="Tokens without an expiration date are not supported"
            severity="info"
            onRemove={() => setNoExpirationInfo(false)}
          >
            Your token was still created, we just used the maximum allowed expiration time instead.
          </Alert>
        )}

        <ServiceAccountTokensTable
          tokens={tokens}
          timeZone="browser"
          onDelete={handleDeleteToken}
          tokenActionsDisabled={false}
        />

        <Button
          onClick={() => setIsCreateModalOpen(true)}
          key="add-service-account-token"
          icon="plus"
          className={s.addButton}
        >
          Add user token
        </Button>

        <CreateTokenModal
          isOpen={isCreateModalOpen}
          token={newToken}
          serviceAccountLogin={TOKEN_NAME_PREFIX}
          onCreateToken={handleCreateToken}
          onClose={() => {
            setNewToken('');
            setIsCreateModalOpen(false);
          }}
        />
      </div>
    </PluginPage>
  );
};

export default TokensPage;

const getStyles = (theme: GrafanaTheme2) => ({
  addButton: css`
    margin-top: ${theme.spacing(2)};
  `,
});
