import React, { useEffect, useState } from 'react';
import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { Button, useStyles2 } from '@grafana/ui';
import { PluginPage } from '@grafana/runtime';
import { getBackendSrv } from '@grafana/runtime';
import { lastValueFrom } from 'rxjs';
import { CreateTokenModal } from './components/CreateTokenModal';
import { ServiceAccountTokensTable } from './components/ServiceAccountTokensTable';

export type Token = {
  id: number;
  name: string;
  key?: string;
  created?: string;
  expiration?: string;
  hasExpired: boolean;
  lastUsedAt?: string;
};

// Type alias so Grafana components work with our Token type
export type ApiKey = Token & {
  isRevoked?: boolean;
  secondsUntilExpiration?: number;
};

// Enrich tokens with computed properties required by Grafana's components
const enrichToken = (token: Token): ApiKey => ({
  ...token,
  isRevoked: false, // OSS doesn't support revocation
  secondsUntilExpiration: token.expiration
    ? Math.floor((new Date(token.expiration).getTime() - Date.now()) / 1000)
    : undefined,
});

const TokensPage = () => {
  const s = useStyles2(getStyles);
  
  // Get plugin ID from URL
  const getPluginId = () => {
    const matches = window.location.pathname.match(/\/api\/plugins\/([^\/]+)\//);
    return matches ? matches[1] : 'joshuagrisham-gcxonpremoauth-app';
  };
  const pluginId = getPluginId();

  const [tokens, setTokens] = useState<ApiKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [newToken, setNewToken] = useState<string>('');

  // Fetch tokens on mount
  useEffect(() => {
    fetchTokens();
  }, []);

  const fetchTokens = async () => {
    try {
      setLoading(true);
      const response = await getBackendSrv().fetch({
        method: 'GET',
        url: `/api/plugins/${pluginId}/resources/tokens`,
      });
      const result = await lastValueFrom(response);
      const enrichedTokens = (result.data as Token[]).map(enrichToken);
      setTokens(enrichedTokens);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch tokens');
    } finally {
      setLoading(false);
    }
  };

  const handleCreateToken = async (data: { name: string; secondsToLive?: number }) => {
    try {
      const response = await getBackendSrv().fetch({
        method: 'POST',
        url: `/api/plugins/${pluginId}/resources/token`,
        data: {
          name: data.name,
          secondsToLive: data.secondsToLive,
        },
      });
      const result = await lastValueFrom(response);
      const createdToken = result.data as Token;

      // Store the token key to display in the modal
      setNewToken(createdToken.key || '');

      // Refresh token list after creation
      await fetchTokens();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create token');
    }
  };

  const handleDeleteToken = async (token: ApiKey) => {
    try {
      const response = await getBackendSrv().fetch({
        method: 'DELETE',
        url: `/api/plugins/${pluginId}/resources/tokens/${token.id}`,
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
        <div>Loading tokens...</div>
      </PluginPage>
    );
  }

  return (
    <PluginPage>
      <div>
        {error && <div className={s.error}>{error}</div>}

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
        >
          Add user token
        </Button>

        <CreateTokenModal
          isOpen={isCreateModalOpen}
          token={newToken}
          serviceAccountLogin={'plugin-app'}
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
  error: css`
    color: ${theme.colors.error.main};
    padding: ${theme.spacing(2)};
    background: ${theme.colors.error.transparent};
    border-radius: ${theme.shape.radius.md};
    margin-bottom: ${theme.spacing(2)};
  `,
});
