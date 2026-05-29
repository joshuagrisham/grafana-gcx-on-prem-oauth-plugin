import React, { useMemo, useState } from 'react';
import { css } from '@emotion/css';
import { GrafanaTheme2, SelectableValue } from '@grafana/data';
import { PluginPage, config } from '@grafana/runtime';
import { ClipboardButton, Field, Input, LinkButton, RadioButtonGroup, useStyles2 } from '@grafana/ui';
import { prefixRoute } from 'utils/utils.routing';
import { ROUTES } from '../constants';

/**
 * "Client setup" page.
 * Pre-fills a per-org "context" name and renders ready-to-copy snippets that
 * wire up gcx, cURL, and the Grafana MCP server against this exact Grafana
 * instance and organization.
 */

const slugify = (input: string): string =>
  input
    .toLowerCase()
    .normalize('NFKD')
    // strip diacritics
    .replace(/[\u0300-\u036f]/g, '')
    // periods, slashes and similar separators become dashes
    .replace(/[._/\\:]+/g, '-')
    // anything that isn't alphanumeric or dash becomes a dash
    .replace(/[^a-z0-9-]+/g, '-')
    // collapse repeats and trim
    .replace(/-+/g, '-')
    .replace(/^-|-$/g, '');

const buildDefaultContextName = (hostname: string, orgName: string, orgId: number): string => {
  const hostSlug = slugify(hostname);
  const orgSlug = orgId > 1 ? slugify(orgName) : '';
  return [hostSlug, orgSlug].filter(Boolean).join('-') || 'grafana';
};

// Strip a trailing slash from a URL while preserving the protocol/host.
const trimTrailingSlash = (url: string): string => url.replace(/\/+$/, '');

const ClientSetupPage = () => {
  const s = useStyles2(getStyles);

  const orgId = config.bootData.user.orgId;
  const orgName = config.bootData.user.orgName;
  // `config.appUrl` is the canonical public root URL configured for this
  // Grafana instance (root_url / GF_SERVER_ROOT_URL). Fall back to
  // window.location.origin if it's empty for any reason.
  const serverUrl = trimTrailingSlash(config.appUrl || window.location.origin);
  const hostname = useMemo(() => {
    try {
      return new URL(serverUrl).hostname;
    } catch {
      return window.location.hostname;
    }
  }, [serverUrl]);

  const defaultContextName = useMemo(
    () => buildDefaultContextName(hostname, orgName, orgId),
    [hostname, orgName, orgId]
  );

  const [contextName, setContextName] = useState(defaultContextName);
  const [mcpClient, setMcpClient] = useState<McpClient>('vscode');
  const [mcpCreds, setMcpCreds] = useState<McpCreds>('hardcoded');
  const [mcpOutput, setMcpOutput] = useState<McpOutput>('raw');
  const [pastedToken, setPastedToken] = useState('');

  // Use the user-typed value when set, otherwise fall back to the default
  // (prevents downstream snippets rendering with an empty key).
  const effectiveContext = contextName.trim() || defaultContextName;

  const snippets = useMemo(
    () =>
      buildSnippets({
        contextName: effectiveContext,
        orgId,
        serverUrl,
        mcpClient,
        mcpCreds,
        mcpOutput,
        pastedToken: pastedToken.trim(),
      }),
    [effectiveContext, orgId, serverUrl, mcpClient, mcpCreds, mcpOutput, pastedToken]
  );

  const mcpFilePath =
    mcpClient === 'vscode' ? (
      <><code>~/.config/Code/User/mcp.json</code> (or <code>.vscode/mcp.json</code> per workspace)</>
    ) : (
      <><code>~/.config/&lt;client&gt;/mcp.json</code> (see your client&apos;s documentation)</>
    );

  return (
    <PluginPage>
      <div className={s.container}>

        <p className={s.intro}>
          This page generates ready-to-copy commands that wire up popular CLI
          clients and the Grafana MCP server against <strong>this</strong>{' '}
          Grafana instance and the organization you are currently viewing.
        </p>

        <div className={s.contextCard}>
          <div className={s.contextRow}>
            <div className={s.metaCell}>
              <div className={s.metaLabel}>Server</div>
              <div className={s.metaValue}><code>{serverUrl}</code></div>
            </div>
            <div className={s.metaCell}>
              <div className={s.metaLabel}>Organization</div>
              <div className={s.metaValue}>
                <code>{orgName}</code> <span className={s.muted}>(id {orgId})</span>
              </div>
            </div>
          </div>

          <Field
            label="Context name"
            description={
              'A short, file-system-friendly identifier used as the key under which ' +
              'client tools will store credentials for this server + organization. ' +
              'The default below is derived from the hostname and organization name; ' +
              'edit it to match a convention you prefer and the snippets will update.'
            }
          >
            <Input
              id="set-me-up-context-name"
              value={contextName}
              placeholder={defaultContextName}
              onChange={(e) => setContextName(e.currentTarget.value)}
              width={60}
            />
          </Field>
        </div>

        <Section
          title="1. Log in with gcx"
          description={
            <>
              <a
                href="https://github.com/joshuagrisham/gcx"
                target="_blank"
                rel="noreferrer noopener"
              >
                gcx
              </a>{' '}
              is the recommended CLI. The command below opens this Grafana
              instance&apos;s OAuth-style authorization page in your browser, mints
              a per-user service-account token, and stores it under the context
              name above.
            </>
          }
          snippet={snippets.gcxLogin}
        />

        <Section
          title="2. Use, switch, or list contexts"
          description="Once logged in you can switch between contexts (e.g. different Grafana servers or orgs) without re-authenticating."
          snippet={snippets.gcxContextUsage}
        />

        <Section
          title="3. Print the token to stdout"
          description={
            <>
              Pipe the token into any tool that wants a bearer credential. The
              snippet uses <code>jq</code> to extract just the token string from
              the gcx config view output.
            </>
          }
          snippet={snippets.gcxPrintToken}
        />

        <Section
          title="4. Call the Grafana HTTP API with cURL"
          description="A minimal example using the token from gcx as a bearer credential against the Grafana API."
          snippet={snippets.curl}
        />

        <Section
          title="5. Configure the Grafana MCP server"
          description={
            <>
              Pick your MCP client, credential style, and output format. The
              snippet updates live based on your selections. Configure your
              clients as follows:
              <ul className={s.compactList}>
                <li>
                  <strong>VS Code</strong>: open the command palette and run{' '}
                  <code>MCP: Open User Configuration</code> (user-wide) or add
                  a <code>.vscode/mcp.json</code> file in your workspace.
                  Schema reference:{' '}
                  <a
                    href="https://code.visualstudio.com/docs/copilot/chat/mcp-servers"
                    target="_blank"
                    rel="noreferrer noopener"
                  >
                    VS Code MCP docs
                  </a>
                  .
                </li>
                <li>
                  <strong>Claude Desktop / Cursor / Continue / Cline</strong>:
                  use the <code>mcpServers</code> schema. See the{' '}
                  <a
                    href="https://modelcontextprotocol.io/quickstart/user"
                    target="_blank"
                    rel="noreferrer noopener"
                  >
                    MCP user quickstart
                  </a>{' '}
                  and the{' '}
                  <a
                    href="https://github.com/grafana/mcp-grafana/blob/main/README.md#usage"
                    target="_blank"
                    rel="noreferrer noopener"
                  >
                    mcp-grafana README
                  </a>
                  .
                </li>
              </ul>
            </>
          }
        >
          <Field label="Client">
            <RadioButtonGroup<McpClient>
              options={MCP_CLIENT_OPTIONS}
              value={mcpClient}
              onChange={(v) => v && setMcpClient(v)}
            />
          </Field>
          <p className={s.filePathNote}>
            {mcpCreds === 'hardcoded' && mcpOutput === 'raw' ? 'Paste into:' : 'Command writes to:'}{' '}
            {mcpFilePath}
          </p>
          <Field label="Credentials">
            <RadioButtonGroup<McpCreds>
              options={MCP_CREDS_OPTIONS}
              value={mcpCreds}
              onChange={(v) => v && setMcpCreds(v)}
            />
          </Field>
          <Field label="Output">
            <RadioButtonGroup<McpOutput>
              options={MCP_OUTPUT_OPTIONS}
              value={mcpOutput}
              onChange={(v) => v && setMcpOutput(v)}
            />
          </Field>
          {mcpCreds === 'hardcoded' && (
            <>
              <Field
                label="Paste your service-account token"
                description={
                  'The value below will be substituted into the snippet wherever a token is needed. ' +
                  'Leave empty to keep the <paste-token-here> placeholder.'
                }
                className={s.toggleField}
              >
                <div className={s.tokenRow}>
                  <Input
                    id="set-me-up-paste-token"
                    value={pastedToken}
                    placeholder="<paste-token-here>"
                    onChange={(e) => setPastedToken(e.currentTarget.value)}
                    type="password"
                    width={60}
                  />
                  <LinkButton
                    href={prefixRoute(ROUTES.Tokens)}
                    variant="secondary"
                    icon="key-skeleton-alt"
                  >
                    Manage user tokens
                  </LinkButton>
                </div>
              </Field>
              <p className={s.warn}>
                This token is held only in your browser and is never sent
                anywhere by this page. While hardcoded tokens are convenient,
                committing them to source control or sharing
                your <code>mcp.json</code> will leak access. Try to use
                the <em>Resolve from gcx</em> variant when possible. 
              </p>
            </>
          )}
          <CodeSnippet
            code={snippets.mcpSnippet}
            language={snippets.mcpLanguage}
          />
          {mcpOutput === 'jq' && (
            <p className={s.muted}>
              Upserts the <code>{effectiveContext}</code> entry into the{' '}
              <code>{mcpClient === 'vscode' ? 'servers' : 'mcpServers'}</code>{' '}
              key without touching the rest of the file. Writes to a temp file
              and renames atomically so a partial write can&apos;t corrupt
              your config.
            </p>
          )}
          {mcpCreds === 'gcx' && mcpOutput === 'raw' && (
            <p className={s.muted}>
              Resolves the token and org-id from your gcx configuration and
              writes the complete MCP config file. Run it again after{' '}
              <code>gcx login</code> to refresh the token.
            </p>
          )}
        </Section>

      </div>
    </PluginPage>
  );
};

export default ClientSetupPage;

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

interface SectionProps {
  title: string;
  description: React.ReactNode;
  snippet?: string;
  children?: React.ReactNode;
}

const Section = ({ title, description, snippet, children }: SectionProps) => {
  const s = useStyles2(getStyles);
  return (
    <section className={s.section}>
      <h3 className={s.sectionTitle}>{title}</h3>
      <p className={s.sectionDescription}>{description}</p>
      {snippet !== undefined && <CodeSnippet code={snippet} />}
      {children}
    </section>
  );
};

interface CodeSnippetProps {
  code: string;
  language?: 'bash' | 'json';
}

const CodeSnippet = ({ code, language = 'bash' }: CodeSnippetProps) => {
  const s = useStyles2(getStyles);
  return (
    <div className={s.snippetWrapper}>
      <div className={s.snippetHeader}>
        <span className={s.snippetLanguage}>{language}</span>
        <ClipboardButton
          icon="copy"
          variant="secondary"
          size="sm"
          fill="text"
          getText={() => code}
        >
          Copy
        </ClipboardButton>
      </div>
      <pre className={s.snippetBody}>
        <code>{code}</code>
      </pre>
    </div>
  );
};

// ---------------------------------------------------------------------------
// Snippet generators
// ---------------------------------------------------------------------------

export type McpClient = 'vscode' | 'mcpServers';
export type McpCreds = 'gcx' | 'hardcoded';
export type McpOutput = 'raw' | 'jq';

const MCP_CLIENT_OPTIONS: Array<SelectableValue<McpClient>> = [
  { label: 'VS Code', value: 'vscode', description: 'servers schema with type: stdio' },
  {
    label: 'Claude Desktop / Cursor / other',
    value: 'mcpServers',
    description: 'mcpServers schema',
  },
];

const MCP_CREDS_OPTIONS: Array<SelectableValue<McpCreds>> = [
  { label: 'Hardcoded', value: 'hardcoded' },
  { label: 'Resolve from gcx at runtime', value: 'gcx' },
];

const MCP_OUTPUT_OPTIONS: Array<SelectableValue<McpOutput>> = [
  { label: 'Raw JSON', value: 'raw', description: 'Drop into a fresh mcp.json' },
  { label: 'Merge with jq', value: 'jq', description: 'Upsert into an existing mcp.json' },
];

interface BuildSnippetArgs {
  contextName: string;
  orgId: number;
  serverUrl: string;
  mcpClient: McpClient;
  mcpCreds: McpCreds;
  mcpOutput: McpOutput;
  pastedToken: string;
}

const buildSnippets = ({
  contextName,
  orgId,
  serverUrl,
  mcpClient,
  mcpCreds,
  mcpOutput,
  pastedToken,
}: BuildSnippetArgs) => {
  const gcxLogin = `gcx login \\
  --context ${contextName} \\
  --server ${serverUrl} \\
  --org-id ${orgId}`;

  const gcxContextUsage = `# List all configured contexts
gcx config list-contexts

# Switch the active context
gcx config use-context ${contextName}

# Show the currently active context
gcx config current-context`;

  const gcxPrintTokenCmd =
    `gcx config view --json contexts --raw \\\n` +
    `  | jq -r '.contexts["${contextName}"].grafana.token'`;

  const gcxPrintToken = `# Prints just the token for the "${contextName}" context to stdout.
${gcxPrintTokenCmd}

# Example: use it inline as a bearer token
TOKEN=$(${gcxPrintTokenCmd.replace(/\\\n\s*/g, '')})
echo "$TOKEN"`;

  const curl = `# Fetch the current user's org membership via the Grafana API.
TOKEN=$(${gcxPrintTokenCmd.replace(/\\\n\s*/g, '')})

curl -sSf \\
  -H "Authorization: Bearer $TOKEN" \\
  -H "X-Grafana-Org-Id: ${orgId}" \\
  ${serverUrl}/api/org`;

  // -------------------------------------------------------------------------
  // MCP snippet generation.
  //
  // Hardcoded: command=docker, env has literal values. Simple and static.
  //
  // gcx: command=bash -c "...", resolves token/org-id from gcx at every MCP
  // server startup. The bash script runs gcx+jq, exports the values, then
  // exec's docker. This works because VS Code spawns the command through a
  // shell when command="bash" + args=["-c", "..."].
  // -------------------------------------------------------------------------

  const serverKey = `grafana-${contextName}`;
  const topLevelKey = mcpClient === 'vscode' ? 'servers' : 'mcpServers';
  const emptyDocument = mcpClient === 'vscode'
    ? '{"servers":{},"inputs":[]}'
    : '{"mcpServers":{}}';
  const defaultPath = mcpClient === 'vscode'
    ? '$HOME/.config/Code/User/mcp.json'
    : '$HOME/.config/<client>/mcp.json';

  const hardcodedTokenValue = pastedToken || '<paste-token-here>';

  // The bash -c script for the gcx variant. Resolves token + org-id then
  // exec's docker with the values injected as -e VAR=value.
  // The \" in the jq filters are JSON-escaped double quotes — they become
  // literal " when the JSON is parsed, which is what bash needs inside the
  // single-quoted jq filter strings.
  const bashCmd = [
    `TOKEN=$(gcx config view --json contexts --raw | jq -r '.contexts[\\"${contextName}\\"].grafana.token');`,
    `ORG_ID=$(gcx config view --json contexts --raw | jq -r '.contexts[\\"${contextName}\\"].grafana[\\"org-id\\"]');`,
    `exec docker run --rm -i`,
    `-e GRAFANA_URL=${serverUrl}`,
    `-e GRAFANA_SERVICE_ACCOUNT_TOKEN=$TOKEN`,
    `-e GRAFANA_ORG_ID=$ORG_ID`,
    `grafana/mcp-grafana -t stdio`,
  ].join(' ');

  let mcpSnippet: string;
  let mcpLanguage: 'json' | 'bash';

  if (mcpCreds === 'hardcoded' && mcpOutput === 'raw') {
    // Pure JSON — user pastes directly into their mcp.json.
    mcpLanguage = 'json';
    const typeJson = mcpClient === 'vscode' ? '\n      "type": "stdio",' : '';
    const inputsJson = mcpClient === 'vscode' ? ',\n  "inputs": []' : '';
    mcpSnippet = `{
  "${topLevelKey}": {
    "${serverKey}": {${typeJson}
      "command": "docker",
      "args": [
        "run", "--rm", "-i",
        "-e", "GRAFANA_URL",
        "-e", "GRAFANA_SERVICE_ACCOUNT_TOKEN",
        "-e", "GRAFANA_ORG_ID",
        "grafana/mcp-grafana",
        "-t", "stdio"
      ],
      "env": {
        "GRAFANA_URL": "${serverUrl}",
        "GRAFANA_SERVICE_ACCOUNT_TOKEN": "${hardcodedTokenValue}",
        "GRAFANA_ORG_ID": "${orgId}"
      }
    }
  }${inputsJson}
}`;
  } else if (mcpCreds === 'gcx' && mcpOutput === 'raw') {
    // gcx + raw: JSON with bash -c wrapper that resolves values at startup.
    mcpLanguage = 'json';
    const typeJson = mcpClient === 'vscode' ? '\n      "type": "stdio",' : '';
    const inputsJson = mcpClient === 'vscode' ? ',\n  "inputs": []' : '';
    mcpSnippet = `{
  "${topLevelKey}": {
    "${serverKey}": {${typeJson}
      "command": "bash",
      "args": ["-c", "${bashCmd}"]
    }
  }${inputsJson}
}`;
  } else if (mcpCreds === 'hardcoded' && mcpOutput === 'jq') {
    // Hardcoded + merge: use jq --arg to safely embed literal values.
    mcpLanguage = 'bash';
    const typeField = mcpClient === 'vscode' ? '\n      type: "stdio",' : '';
    const jqArgs = `--arg url "${serverUrl}" --arg token "${hardcodedTokenValue}" --arg orgId "${orgId}"`;
    const jqEntry = `{${typeField}
      command: "docker",
      args: ["run","--rm","-i","-e","GRAFANA_URL","-e","GRAFANA_SERVICE_ACCOUNT_TOKEN","-e","GRAFANA_ORG_ID","grafana/mcp-grafana","-t","stdio"],
      env: { GRAFANA_URL: $url, GRAFANA_SERVICE_ACCOUNT_TOKEN: $token, GRAFANA_ORG_ID: $orgId }
    }`;
    mcpSnippet = `MCP_FILE="${defaultPath}"   # adjust for macOS / Windows

# Ensure the file exists and contains valid JSON; reset if not.
[ -s "$MCP_FILE" ] && jq empty "$MCP_FILE" 2>/dev/null || echo '${emptyDocument}' > "$MCP_FILE"

jq ${jqArgs} '
  .${topLevelKey}["${serverKey}"] = ${jqEntry}
' "$MCP_FILE" > "$MCP_FILE.tmp" && mv "$MCP_FILE.tmp" "$MCP_FILE"`;
  } else {
    // gcx + merge: upsert the bash-c wrapper entry via jq -s + heredoc.
    mcpLanguage = 'bash';
    const typeJson = mcpClient === 'vscode' ? '\n      "type": "stdio",' : '';
    const inputsJson = mcpClient === 'vscode' ? ',\n  "inputs": []' : '';
    const gcxJson = `{
  "${topLevelKey}": {
    "${serverKey}": {${typeJson}
      "command": "bash",
      "args": ["-c", "${bashCmd}"]
    }
  }${inputsJson}
}`;
    mcpSnippet = `MCP_FILE="${defaultPath}"   # adjust for macOS / Windows

# Ensure the file exists and contains valid JSON; reset if not.
[ -s "$MCP_FILE" ] && jq empty "$MCP_FILE" 2>/dev/null || echo '${emptyDocument}' > "$MCP_FILE"

jq -s '.[1].${topLevelKey}["${serverKey}"] as $e | .[0] | .${topLevelKey}["${serverKey}"] = $e' \\
   "$MCP_FILE" - > "$MCP_FILE.tmp" <<'JSON'
${gcxJson}
JSON
mv "$MCP_FILE.tmp" "$MCP_FILE"`;
  }

  return { gcxLogin, gcxContextUsage, gcxPrintToken, curl, mcpSnippet, mcpLanguage };
};

// ---------------------------------------------------------------------------
// Styles
// ---------------------------------------------------------------------------

const getStyles = (theme: GrafanaTheme2) => ({
  container: css`
    display: flex;
    flex-direction: column;
    gap: ${theme.spacing(3)};
    max-width: 960px;
    padding-top: ${theme.spacing(2)};
  `,
  intro: css`
    color: ${theme.colors.text.secondary};
    margin: 0;
  `,
  contextCard: css`
    background: ${theme.colors.background.secondary};
    border: 1px solid ${theme.colors.border.weak};
    border-radius: ${theme.shape.radius.default};
    padding: ${theme.spacing(2)};
    display: flex;
    flex-direction: column;
    gap: ${theme.spacing(2)};
  `,
  contextRow: css`
    display: flex;
    gap: ${theme.spacing(4)};
    flex-wrap: wrap;
  `,
  metaCell: css`
    display: flex;
    flex-direction: column;
    gap: ${theme.spacing(0.25)};
  `,
  metaLabel: css`
    color: ${theme.colors.text.secondary};
    font-size: ${theme.typography.bodySmall.fontSize};
    text-transform: uppercase;
    letter-spacing: 0.04em;
  `,
  metaValue: css`
    color: ${theme.colors.text.primary};
  `,
  muted: css`
    color: ${theme.colors.text.secondary};
  `,
  filePathNote: css`
    color: ${theme.colors.text.secondary};
    font-size: ${theme.typography.bodySmall.fontSize};
    margin: -${theme.spacing(1)} 0 ${theme.spacing(2)};
  `,
  section: css`
    display: flex;
    flex-direction: column;
    gap: ${theme.spacing(1)};
  `,
  sectionTitle: css`
    margin: 0;
  `,
  sectionDescription: css`
    color: ${theme.colors.text.secondary};
    margin: 0;
  `,
  tabs: css`
    margin-bottom: ${theme.spacing(1)};
  `,
  toggleRow: css`
    display: flex;
    gap: ${theme.spacing(3)};
    flex-wrap: wrap;
  `,
  toggleField: css`
    margin-bottom: 0;
  `,
  tokenRow: css`
    display: flex;
    align-items: center;
    gap: ${theme.spacing(1)};
    flex-wrap: wrap;
  `,
  compactList: css`
    margin: ${theme.spacing(0.5, 0, 0, 0)};
    padding-left: ${theme.spacing(3)};

    & > li + li {
      margin-top: ${theme.spacing(0.5)};
    }
  `,
  warn: css`
    color: ${theme.colors.warning.text};
    background: ${theme.colors.warning.transparent};
    border: 1px solid ${theme.colors.warning.borderTransparent};
    border-radius: ${theme.shape.radius.default};
    padding: ${theme.spacing(1, 1.5)};
    margin: 0 0 ${theme.spacing(1)} 0;
    font-size: ${theme.typography.bodySmall.fontSize};
  `,
  snippetWrapper: css`
    border: 1px solid ${theme.colors.border.weak};
    border-radius: ${theme.shape.radius.default};
    background: ${theme.colors.background.canvas};
    overflow: hidden;
  `,
  snippetHeader: css`
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: ${theme.spacing(0.5, 1)};
    background: ${theme.colors.background.secondary};
    border-bottom: 1px solid ${theme.colors.border.weak};
  `,
  snippetLanguage: css`
    color: ${theme.colors.text.secondary};
    font-size: ${theme.typography.bodySmall.fontSize};
    text-transform: uppercase;
    letter-spacing: 0.05em;
  `,
  snippetBody: css`
    margin: 0;
    padding: ${theme.spacing(1.5, 2)};
    font-family: ${theme.typography.fontFamilyMonospace};
    font-size: ${theme.typography.bodySmall.fontSize};
    color: ${theme.colors.text.primary};
    overflow-x: auto;
    white-space: pre;
  `,
});
