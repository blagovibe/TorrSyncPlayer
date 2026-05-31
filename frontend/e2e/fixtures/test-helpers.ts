import { Page, expect } from '@playwright/test';

/**
 * Вспомогательные функции для E2E тестов
 */

/**
 * Переход в режим хоста (создание комнаты)
 */
export async function goToHostMode(page: Page): Promise<void> {
  await page.locator('button.btn-host').click();
  await expect(page.locator('h2.section-title:has-text("Создать комнату")')).toBeVisible();
}

/**
 * Переход в режим гостя (присоединение к комнате)
 */
export async function goToGuestMode(page: Page): Promise<void> {
  await page.locator('button.btn-guest').click();
  await expect(page.locator('h2.section-title:has-text("Присоединиться к комнате")')).toBeVisible();
}

/**
 * Возврат на главную страницу
 */
export async function goBack(page: Page): Promise<void> {
  await page.locator('button:has-text("← Назад")').first().click();
  await expect(page.locator('button.btn-host')).toBeVisible();
  await expect(page.locator('button.btn-guest')).toBeVisible();
}

/**
 * Заполнение формы создания комнаты
 */
export async function fillHostForm(page: Page, magnetURI: string, password?: string): Promise<void> {
  const magnetInput = page.locator('input[placeholder*="магнет"]');
  await magnetInput.fill(magnetURI);
  
  if (password) {
    const passwordInput = page.locator('input[type="password"]');
    await passwordInput.fill(password);
  }
}

/**
 * Заполнение формы присоединения к комнате
 */
export async function fillGuestForm(page: Page, roomCode: string): Promise<void> {
  const roomCodeInput = page.locator('input[placeholder*="ID комнаты"]');
  await roomCodeInput.fill(roomCode);
}

/**
 * Проверка что мы на главной странице
 */
export async function assertOnHomePage(page: Page): Promise<void> {
  await expect(page.locator('h1.homepage-title')).toBeVisible();
  await expect(page.locator('h1.homepage-title')).toHaveText('TorrSyncPlayer');
}
