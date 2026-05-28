import { expect, test } from './fixtures';

test.describe('App config page', () => {
  test('renders the token field and submit button', async ({ appConfigPage, page }) => {
    await expect(page.getByRole('heading', { name: 'Environment variables' })).toBeVisible();
    await expect(
      page.getByRole('group', { name: /organization service account token/i })
    ).toBeVisible();
    await expect(page.getByPlaceholder('glsa_...')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Save token' })).toBeVisible();
  });

  test('saving an empty token does not change anything and returns 200', async ({
    appConfigPage,
    page,
  }) => {
    // With no token configured yet, the submit button is enabled but the
    // request body omits secureJsonData; Grafana should still accept it.
    const saveResponse = appConfigPage.waitForSettingsResponse();
    await page.getByRole('button', { name: 'Save token' }).click();
    await expect(saveResponse).toBeOK();
  });
});
