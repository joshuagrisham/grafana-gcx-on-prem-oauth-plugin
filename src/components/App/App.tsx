import React from 'react';
import { Route, Routes } from 'react-router-dom';
import { AppRootProps } from '@grafana/data';
import { ROUTES } from '../../constants';
const MenuPage = React.lazy(() => import('../../pages/MenuPage'));
const AuthorizePage = React.lazy(() => import('../../pages/AuthorizePage'));
const ClientSetupPage = React.lazy(() => import('../../pages/ClientSetupPage'));
const TokensPage = React.lazy(() => import('../../pages/TokensPage'));

function App(props: AppRootProps) {
  return (
    <Routes>
      <Route path={ROUTES.Menu} element={<MenuPage />} />
      <Route path={ROUTES.Authorize} element={<AuthorizePage />} />
      <Route path={ROUTES.Tokens} element={<TokensPage />} />
      <Route path={ROUTES.ClientSetup} element={<ClientSetupPage />} />

      {/* Default page */}
      <Route path="*" element={<MenuPage />} />
    </Routes>
  );
}

export default App;
