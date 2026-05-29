import { expect, test } from './fixtures';
import { ROUTES } from '../src/constants';

test.describe('App navigation', () => {
  test('renders the menu page at the menu route', async ({ gotoPage, page }) => {
    await gotoPage(`/${ROUTES.Menu}`);
    await expect(page.getByText('Manage user tokens')).toBeVisible();
  });

  test('renders the tokens page at the tokens route', async ({ gotoPage, page }) => {
    await gotoPage(`/${ROUTES.Tokens}`);
    // The page shows either the tokens table heading/Create button, or a
    // loading indicator briefly; assert the New token button is reachable.
    await expect(page.getByRole('button', { name: /add user token/i })).toBeVisible();
  });

  test('falls back to the menu page on an unknown subroute', async ({ gotoPage, page }) => {
    await gotoPage('/does-not-exist');
    await expect(page.getByText('Manage user tokens')).toBeVisible();
  });
});
