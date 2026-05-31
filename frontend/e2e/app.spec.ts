import { test, expect } from '@playwright/test';

test.describe('TorrSyncPlayer App', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/');
  });

  test('should load the main page', async ({ page }) => {
    // Проверяем что заголовок страницы загрузился
    await expect(page.locator('h1.homepage-title')).toBeVisible();
    await expect(page.locator('h1.homepage-title')).toHaveText('TorrSyncPlayer');
    
    // Проверяем подзаголовок
    await expect(page.locator('p.homepage-subtitle')).toBeVisible();
    await expect(page.locator('p.homepage-subtitle')).toHaveText('Смотрите вместе в реальном времени');
  });

  test('should display mode selection buttons', async ({ page }) => {
    // Проверяем наличие кнопок выбора режима
    const createRoomBtn = page.locator('button.btn-host');
    const joinRoomBtn = page.locator('button.btn-guest');
    
    await expect(createRoomBtn).toBeVisible();
    await expect(createRoomBtn).toHaveText(/Создать комнату/);
    
    await expect(joinRoomBtn).toBeVisible();
    await expect(joinRoomBtn).toHaveText(/Присоединиться/);
  });

  test('should switch to host mode when clicking create room', async ({ page }) => {
    // Нажимаем кнопку создания комнаты
    await page.locator('button.btn-host').click();
    
    // Проверяем что появилась форма создания комнаты
    await expect(page.locator('h2.section-title')).toBeVisible();
    await expect(page.locator('h2.section-title')).toHaveText('Создать комнату');
    
    // Проверяем наличие полей ввода
    const magnetInput = page.locator('input[placeholder*="магнет"]');
    const passwordInput = page.locator('input[type="password"]');
    
    await expect(magnetInput).toBeVisible();
    await expect(passwordInput).toBeVisible();
    
    // Проверяем наличие кнопки "Создать и смотреть"
    const createAndWatchBtn = page.locator('button.btn-host:has-text("Создать и смотреть")');
    await expect(createAndWatchBtn).toBeVisible();
  });

  test('should switch to guest mode when clicking join room', async ({ page }) => {
    // Нажимаем кнопку присоединения к комнате
    await page.locator('button.btn-guest').click();
    
    // Проверяем что появилась форма присоединения
    await expect(page.locator('h2.section-title')).toBeVisible();
    await expect(page.locator('h2.section-title')).toHaveText('Присоединиться к комнате');
    
    // Проверяем наличие поля ввода ID комнаты
    const roomCodeInput = page.locator('input[placeholder*="ID комнаты"]');
    await expect(roomCodeInput).toBeVisible();
    
    // Проверяем наличие кнопки "Присоединиться"
    const joinBtn = page.locator('button.btn-guest:has-text("Присоединиться")');
    await expect(joinBtn).toBeVisible();
  });

  test('should navigate back from host mode', async ({ page }) => {
    // Переходим в режим хоста
    await page.locator('button.btn-host').click();
    
    // Нажимаем кнопку "Назад"
    await page.locator('button:has-text("← Назад")').first().click();
    
    // Проверяем что вернулись к выбору режима
    await expect(page.locator('button.btn-host')).toBeVisible();
    await expect(page.locator('button.btn-guest')).toBeVisible();
  });

  test('should navigate back from guest mode', async ({ page }) => {
    // Переходим в режим гостя
    await page.locator('button.btn-guest').click();
    
    // Нажимаем кнопку "Назад"
    await page.locator('button:has-text("← Назад")').first().click();
    
    // Проверяем что вернулись к выбору режима
    await expect(page.locator('button.btn-host')).toBeVisible();
    await expect(page.locator('button.btn-guest')).toBeVisible();
  });

  test('should accept magnet URI input in host mode', async ({ page }) => {
    // Переходим в режим хоста
    await page.locator('button.btn-host').click();
    
    // Вводим тестовую magnet-ссылку
    const magnetInput = page.locator('input[placeholder*="магнет"]');
    const testMagnet = 'magnet:?xt=urn:btih:test123';
    await magnetInput.fill(testMagnet);
    
    // Проверяем что значение введено
    await expect(magnetInput).toHaveValue(testMagnet);
  });

  test('should accept room code input in guest mode', async ({ page }) => {
    // Переходим в режим гостя
    await page.locator('button.btn-guest').click();
    
    // Вводим тестовый ID комнаты
    const roomCodeInput = page.locator('input[placeholder*="ID комнаты"]');
    const testRoomCode = 'ABC123';
    await roomCodeInput.fill(testRoomCode);
    
    // Проверяем что значение введено
    await expect(roomCodeInput).toHaveValue(testRoomCode);
  });

  test('should accept password input in host mode', async ({ page }) => {
    // Переходим в режим хоста
    await page.locator('button.btn-host').click();
    
    // Вводим тестовый пароль
    const passwordInput = page.locator('input[type="password"]');
    const testPassword = 'testpass123';
    await passwordInput.fill(testPassword);
    
    // Проверяем что значение введено (пароль должен быть скрыт)
    await expect(passwordInput).toHaveValue(testPassword);
  });
});

test.describe('Navigation Flow', () => {
  test('should complete full navigation flow: home -> host -> back -> guest -> back', async ({ page }) => {
    await page.goto('/');
    
    // Находимся на главной
    await expect(page.locator('h1.homepage-title')).toBeVisible();
    
    // Переходим в режим хоста
    await page.locator('button.btn-host').click();
    await expect(page.locator('h2.section-title:has-text("Создать комнату")')).toBeVisible();
    
    // Возвращаемся назад
    await page.locator('button:has-text("← Назад")').first().click();
    await expect(page.locator('button.btn-host')).toBeVisible();
    
    // Переходим в режим гостя
    await page.locator('button.btn-guest').click();
    await expect(page.locator('h2.section-title:has-text("Присоединиться к комнате")')).toBeVisible();
    
    // Возвращаемся назад
    await page.locator('button:has-text("← Назад")').first().click();
    await expect(page.locator('button.btn-host')).toBeVisible();
    await expect(page.locator('button.btn-guest')).toBeVisible();
  });
});
