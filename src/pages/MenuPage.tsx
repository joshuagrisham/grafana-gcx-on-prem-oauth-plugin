import React from 'react';
import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { Icon, useStyles2 } from '@grafana/ui';
import { PluginPage } from '@grafana/runtime';
import { prefixRoute } from '../utils/utils.routing';
import { ROUTES } from '../constants';

interface MenuLink {
  label: string;
  description: string;
  href: string;
  icon?: 'key-skeleton-alt' | 'cog' | 'book';
}

// Add new entries here as additional pages are introduced (e.g. usage
// instructions, example Grafana MCP config). The page renders them as a
// simple list so a single-item menu remains coherent as it grows.
const MENU_LINKS: MenuLink[] = [
  {
    label: 'Manage user tokens',
    description: "View, create and revoke tokens for this user's service account.",
    href: prefixRoute(ROUTES.Tokens),
    icon: 'key-skeleton-alt',
  },
];

function MenuPage() {
  const s = useStyles2(getStyles);

  return (
    <PluginPage>
      <ul className={s.list}>
        {MENU_LINKS.map((link) => (
          <li key={link.href} className={s.item}>
            <a href={link.href} className={s.link}>
              {link.icon && <Icon name={link.icon} className={s.icon} />}
              <div>
                <div className={s.title}>{link.label}</div>
                <div className={s.description}>{link.description}</div>
              </div>
            </a>
          </li>
        ))}
      </ul>
    </PluginPage>
  );
}

export default MenuPage;

const getStyles = (theme: GrafanaTheme2) => ({
  list: css`
    list-style: none;
    padding: 0;
    margin: ${theme.spacing(2)} 0 0 0;
    display: flex;
    flex-direction: column;
    gap: ${theme.spacing(1)};
    max-width: 640px;
  `,
  item: css`
    background: ${theme.colors.background.secondary};
    border: 1px solid ${theme.colors.border.weak};
    border-radius: ${theme.shape.radius.default};
  `,
  link: css`
    display: flex;
    align-items: center;
    gap: ${theme.spacing(2)};
    padding: ${theme.spacing(2)};
    color: ${theme.colors.text.primary};
    text-decoration: none;

    &:hover {
      background: ${theme.colors.action.hover};
    }
  `,
  icon: css`
    color: ${theme.colors.text.secondary};
  `,
  title: css`
    font-weight: ${theme.typography.fontWeightMedium};
  `,
  description: css`
    color: ${theme.colors.text.secondary};
    font-size: ${theme.typography.bodySmall.fontSize};
  `,
});
