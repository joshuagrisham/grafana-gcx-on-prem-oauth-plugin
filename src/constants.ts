import pluginJson from './plugin.json';

export const PLUGIN_ID = pluginJson.id;
export const PLUGIN_BASE_URL = `/a/${PLUGIN_ID}`;
export const PLUGIN_RESOURCES_URL = `/api/plugins/${PLUGIN_ID}/resources`;
export const PLUGIN_ENV_VAR_PREFIX = `GF_PLUGIN_${PLUGIN_ID.replaceAll('-', '_').toUpperCase()}`;

export enum ROUTES {
  Authorize = 'authorize',
  Menu = 'menu',
  Tokens = 'tokens',
  ClientSetup = 'client-setup',
}
