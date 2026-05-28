import React, { ChangeEvent, useState } from 'react';
import { lastValueFrom } from 'rxjs';
import { css } from '@emotion/css';
import { AppPluginMeta, GrafanaTheme2, PluginConfigPageProps, PluginMeta } from '@grafana/data';
import { getBackendSrv } from '@grafana/runtime';
import { Button, Field, FieldSet, SecretInput, useStyles2 } from '@grafana/ui';
import { PLUGIN_ENV_VAR_PREFIX } from '../../constants';

type AppPluginSettings = {
  token?: string;  
};

type State = {
  // The Organization Service Account token that will be used to create all of the per-user Service Accounts and their tokens.
  token: string;
  // Tells us if the token has been set.
  isTokenSet: boolean;
  // Tracks if user has clicked reset and is in edit mode.
  isTokenReset: boolean;
};

export interface AppConfigProps extends PluginConfigPageProps<AppPluginMeta<AppPluginSettings>> {}

const AppConfig = ({ plugin }: AppConfigProps) => {
  const s = useStyles2(getStyles);
  const { enabled, pinned, secureJsonFields } = plugin.meta;
  const [state, setState] = useState<State>({
    token: '',
    isTokenSet: Boolean(secureJsonFields?.token),
    isTokenReset: false,
  });

  const isSubmitDisabled = state.isTokenSet;

  const onResetToken = () =>
    setState({
      ...state,
      token: '',
      isTokenSet: false,
      isTokenReset: true,
    });

  const onChange = (event: ChangeEvent<HTMLInputElement>) => {
    setState({
      ...state,
      [event.target.name]: event.target.value.trim(),
    });
  };

  const onSubmit = (event?: React.FormEvent<HTMLFormElement>) => {
    if (event) {
      event.preventDefault();
    }
    if (isSubmitDisabled) {
      return;
    }

    updatePluginAndReload(plugin.meta.id, {
      enabled,
      pinned,
      jsonData: {},
      // This cannot be queried later by the frontend.
      // We don't want to override it in case it was set previously and left untouched now.
      secureJsonData: state.isTokenSet
        ? undefined
        : state.isTokenReset
          ? { token: state.token }
          : state.token 
            ? { token: state.token }
            : undefined,
    });
  };

  return (
    <form onSubmit={onSubmit}>

      <h2>Environment variables</h2>

      <p className={s.colorWeak}>
        The plugin reads the following environment variables from the Grafana server process. To
        make them available to the plugin, ensure that <code>{plugin.meta.id}</code> is included in
        the <code>forward_host_env_vars</code> setting.
      </p>

      <ul className={`${s.colorWeak} ${s.list}`}>
        <li>
          <code>{PLUGIN_ENV_VAR_PREFIX}_REQUEST_TIMEOUT</code> Per-request timeout applied to all
          outbound Grafana API calls made by the plugin. Defaults to <code>30s</code>.
        </li>
        <li>
          <code>{PLUGIN_ENV_VAR_PREFIX}_BACKEND_USERNAME</code> Basic Auth username to be used by
          the plugin's backend service for authenticating to Grafana's API. See below for more
          information.
        </li>
        <li>
          <code>{PLUGIN_ENV_VAR_PREFIX}_BACKEND_PASSWORD</code> Basic Auth password to be used by
          the plugin's backend service for authenticating to Grafana's API. See below for more
          information.
        </li>
        <li>
          <code>{PLUGIN_ENV_VAR_PREFIX}_MAX_TOKENS_PER_USER</code> Maximum number of concurrently
          active tokens per user. New token creations beyond this limit are rejected. Defaults to
          <code>20</code>. Set to <code>0</code> to disable.
        </li>
        <li>
          <code>{PLUGIN_ENV_VAR_PREFIX}_TOKEN_MAX_SECONDS_TO_LIVE</code> Maximum allowed token
          lifetime (in seconds) for tokens created by this plugin. Defaults to <code>2592000</code>
          (30 days).
        </li>
        <li>
          <code>{PLUGIN_ENV_VAR_PREFIX}_TOKEN_CLEANUP_GRACE_PERIOD</code> Grace period for expired
          tokens before they are automatically deleted by the plugin's background cleanup process.
          Defaults to <code>72h</code> (3 days).
        </li>
        <li>
          <code>{PLUGIN_ENV_VAR_PREFIX}_CLEANUP_INTERVAL</code> How often the background cleanup
          process runs. The background process cleans up expired tokens, removes service accounts
          whose user is gone or disabled, and syncs role changes. Defaults to <code>1h</code>. Set
          to <code>0</code> to disable.
        </li>
      </ul>

      <h2>Backend service Grafana API authentication</h2>

      <p className={s.colorWeak}>
        The plugin's backend service must authenticate to Grafana's API to create per-user service
        accounts and their tokens. As the plugin needs the ability to assign roles to the user
        service accounts it creates, it is not possible to use Grafana's managed plugin service
        account feature. You can either:
      </p>

      <ul className={`${s.colorWeak} ${s.list}`}>
        <li>
          Create a new Service Account in each Organization with the <code>Admin</code> role and
          provide its token in the form below, or
        </li>
        <li>
          Configure Grafana to allow this plugin to use a Basic Auth user with the
          <code>GrafanaAdmin</code> role, enabling it to provision user Service Accounts and tokens
          in all Organizations.
        </li>
      </ul>

      <p className={s.colorWeak}>
        If you prefer to use a <code>GrafanaAdmin</code> user with Basic Auth, configure your
        Grafana instance as follows:
      </p>

      <ul className={`${s.colorWeak} ${s.list}`}>
        <li>
          Enable Basic Auth: set <code>auth.basic.enabled</code> to <code>true</code> (or
          <code>GF_AUTH_BASIC_ENABLED=true</code>).
        </li>
        <li>
          Add <code>{plugin.meta.id}</code> to <code>forward_host_env_vars</code>.
        </li>
        <li>
          Set <code>{PLUGIN_ENV_VAR_PREFIX}_BACKEND_USERNAME</code> and <code>{PLUGIN_ENV_VAR_PREFIX}_BACKEND_PASSWORD</code> to
          the credentials of your <code>GrafanaAdmin</code> user.
        </li>
      </ul>

      <FieldSet label="Organization Service Account Token">

        <p className={s.colorWeak}>
          Provide a token here if you want the plugin's backend service to use it when
          authenticating to Grafana's API for creating per-user Service Accounts and tokens within
          this Organization.
        </p>

        <p className={s.colorWeak}>
          This is required unless your Grafana instance is configured to use Basic Auth for this
          plugin as described above.
        </p>

        <Field label="Token">
          <SecretInput
            width={60}
            id="config-token"
            name="token"
            value={state.token}
            isConfigured={state.isTokenSet}
            placeholder={'glsa_...'}
            onChange={onChange}
            onReset={onResetToken}
          />
        </Field>

      </FieldSet>

      <div className={s.marginTop}>
        <Button type="submit" disabled={isSubmitDisabled}>
          Save token
        </Button>
      </div>

    </form>
  );
};

export default AppConfig;

const getStyles = (theme: GrafanaTheme2) => ({
  colorWeak: css`
    color: ${theme.colors.text.secondary};
  `,
  marginTop: css`
    margin-top: ${theme.spacing(3)};
  `,
  list: css`
    padding-left: ${theme.spacing(3)};
    margin-bottom: ${theme.spacing(3)};
  `,
});

const updatePluginAndReload = async (pluginId: string, data: Partial<PluginMeta<AppPluginSettings>>) => {
  try {
    await updatePlugin(pluginId, data);

    // Reloading the page as the changes made here wouldn't be propagated to the actual plugin otherwise.
    // This is not ideal, however unfortunately currently there is no supported way for updating the plugin state.
    window.location.reload();
  } catch (e) {
    console.error('Error while updating the plugin', e);
  }
};

const updatePlugin = async (pluginId: string, data: Partial<PluginMeta>) => {
  const response = await getBackendSrv().fetch({
    url: `/api/plugins/${pluginId}/settings`,
    method: 'POST',
    data,
  });

  return lastValueFrom(response);
};
