import React from 'react';
import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { LinkButton, useStyles2 } from '@grafana/ui';
import { prefixRoute } from '../utils/utils.routing';
import { ROUTES } from '../constants';
import { PluginPage } from '@grafana/runtime';

function MenuPage() {
  const s = useStyles2(getStyles);

  return (
    <PluginPage>
      <div>
        <div className={s.marginTop}>
          <LinkButton href={prefixRoute(ROUTES.Tokens)}>
            Manage user tokens
          </LinkButton>
        </div>
      </div>
    </PluginPage>
  );
}

export default MenuPage;

const getStyles = (theme: GrafanaTheme2) => ({
  marginTop: css`
    margin-top: ${theme.spacing(2)};
  `,
});
