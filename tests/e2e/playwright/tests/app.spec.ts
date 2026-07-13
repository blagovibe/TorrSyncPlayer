import { test, expect, Page } from '@playwright/test';

const TEST_USER = {
  username: 'e2etestuser',
  password: 'TestPass123!',
};

test.describe('TorrSyncPlayer E2E Tests', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('https://localhost:8889', { waitUntil: 'networkidle' });
  });

  test.describe('Authentication', () => {
    test('should show login form', async ({ page }) => {
      await expect(page.locator('input[name="username"]')).toBeVisible();
      await expect(page.locator('input[name="password"]')).toBeVisible();
      await expect(page.locator('button[type="submit"]')).toContainText('Login');
    });

    test('should register new user', async ({ page }) => {
      await page.fill('input[name="username"]', TEST_USER.username);
      await page.fill('input[name="password"]', TEST_USER.password);
      await page.click('button:has-text("Register")');
      
      await expect(page.locator('.toast, .notification, .alert')).toContainText(/success|registered|created/i, { timeout: 10000 });
    });

    test('should login existing user', async ({ page }) => {
      // Register first if needed
      await page.fill('input[name="username"]', TEST_USER.username);
      await page.fill('input[name="password"]', TEST_USER.password);
      await page.click('button:has-text("Register")');
      await expect(page.locator('.toast, .notification')).toContainText(/success/i, { timeout: 10000 });
      
      // Logout if logged in
      await page.click('button:has-text("Logout"), a:has-text("Logout")').catch(() => {});
      
      // Login
      await page.fill('input[name="username"]', TEST_USER.username);
      await page.fill('input[name="password"]', TEST_USER.password);
      await page.click('button:has-text("Login")');
      
      // Should redirect to main app
      await expect(page.locator('.main-window, .app-container, .torrent-list')).toBeVisible({ timeout: 10000 });
    });
  });

  test.describe('Torrent Management', () => {
    test.beforeEach(async ({ page }) => {
      // Login
      await page.fill('input[name="username"]', TEST_USER.username);
      await page.fill('input[name="password"]', TEST_USER.password);
      await page.click('button:has-text("Login")');
      await expect(page.locator('.main-window, .app-container')).toBeVisible({ timeout: 10000 });
    });

    test('should add torrent via magnet link', async ({ page }) => {
      const magnetInput = page.locator('input[placeholder*="magnet"], input[name*="magnet"], input[placeholder*="Magnet"]');
      await expect(magnetInput).toBeVisible({ timeout: 5000 });
      
      await magnetInput.fill('magnet:?xt=urn:btih:abcdef1234567890abcdef1234567890abcdef12&dn=Test+Video');
      await page.click('button:has-text("Add"), button:has-text("Add Torrent")');
      
      // Should show torrent in list
      await expect(page.locator('.torrent-item, .torrent-row, [data-testid="torrent-list"]')).toBeVisible({ timeout: 15000 });
    });

    test('should list torrents', async ({ page }) => {
      // Navigate to torrent list if not already there
      await page.click('a:has-text("Torrents"), button:has-text("Torrents"), nav a:has-text("Torrents")').catch(() => {});
      
      // Should show torrent list (even if empty)
      await expect(page.locator('.torrent-list, .torrents-container, [data-testid="torrent-list"]')).toBeVisible({ timeout: 5000 });
    });

    test('should remove torrent', async ({ page }) => {
      // Add a torrent first
      const magnetInput = page.locator('input[placeholder*="magnet"], input[name*="magnet"]');
      await magnetInput.fill('magnet:?xt=urn:btih:1111111111111111111111111111111111111111&dn=ToRemove');
      await page.click('button:has-text("Add"), button:has-text("Add Torrent")');
      
      await expect(page.locator('.torrent-item')).toBeVisible({ timeout: 15000 });
      
      // Remove it
      await page.locator('.torrent-item').first().hover();
      await page.click('button:has-text("Remove"), button:has-text("Delete"), [aria-label*="remove"], [aria-label*="delete"]');
      
      // Confirm if needed
      await page.click('button:has-text("Confirm"), button:has-text("Yes"), button:has-text("Delete")').catch(() => {});
      
      // Should be removed
      await expect(page.locator('.torrent-item')).not.toBeVisible({ timeout: 5000 });
    });
  });

  test.describe('P2P Room Operations', () => {
    test.beforeEach(async ({ page }) => {
      // Login
      await page.fill('input[name="username"]', TEST_USER.username);
      await page.fill('input[name="password"]', TEST_USER.password);
      await page.click('button:has-text("Login")');
      await expect(page.locator('.main-window, .app-container')).toBeVisible({ timeout: 10000 });
    });

    test('should create new room', async ({ page }) => {
      // Navigate to rooms
      await page.click('a:has-text("Rooms"), button:has-text("Rooms"), nav a:has-text("Rooms")').catch(() => {});
      
      // Create room
      await page.click('button:has-text("Create Room"), button:has-text("New Room")');
      await page.fill('input[name="roomName"], input[placeholder*="room name"], input[placeholder*="Room name"]', 'E2E Test Room');
      await page.click('button:has-text("Create"), button[type="submit"]');
      
      // Should show room created
      await expect(page.locator('.room-item, .room-card, [data-testid="room-list"]')).toContainText('E2E Test Room', { timeout: 10000 });
    });

    test('should join room', async ({ page }) => {
      // Create room first
      await page.click('button:has-text("Create Room"), button:has-text("New Room")');
      await page.fill('input[name="roomName"], input[placeholder*="room name"]', 'Join Test Room');
      await page.click('button:has-text("Create"), button[type="submit"]');
      await expect(page.locator('.room-item')).toContainText('Join Test Room', { timeout: 10000 });
      
      // Click join
      await page.locator('.room-item:has-text("Join Test Room") button:has-text("Join")').click();
      
      // Should show room interface
      await expect(page.locator('.room-interface, .peer-list, .sync-controls')).toBeVisible({ timeout: 10000 });
    });

    test('should leave room', async ({ page }) => {
      // Create and join room
      await page.click('button:has-text("Create Room")');
      await page.fill('input[placeholder*="room name"]', 'Leave Test Room');
      await page.click('button:has-text("Create")');
      await expect(page.locator('.room-item')).toContainText('Leave Test Room', { timeout: 10000 });
      await page.locator('.room-item:has-text("Leave Test Room") button:has-text("Join")').click();
      
      // Leave room
      await page.click('button:has-text("Leave"), button:has-text("Leave Room")');
      
      // Should return to room list
      await expect(page.locator('.room-list, .rooms-container')).toBeVisible({ timeout: 5000 });
    });
  });

  test.describe('Playback Synchronization', () => {
    test.beforeEach(async ({ page }) => {
      // Login
      await page.fill('input[name="username"]', TEST_USER.username);
      await page.fill('input[name="password"]', TEST_USER.password);
      await page.click('button:has-text("Login")');
      await expect(page.locator('.main-window')).toBeVisible({ timeout: 10000 });
      
      // Create and join room
      await page.click('button:has-text("Create Room")');
      await page.fill('input[placeholder*="room name"]', 'Sync Test Room');
      await page.click('button:has-text("Create")');
      await expect(page.locator('.room-item')).toContainText('Sync Test Room', { timeout: 10000 });
      await page.locator('.room-item:has-text("Sync Test Room") button:has-text("Join")').click();
      await expect(page.locator('.sync-controls')).toBeVisible({ timeout: 10000 });
    });

    test('should sync play', async ({ page }) => {
      await page.click('button:has-text("Play"), button[aria-label*="play"], .sync-play');
      
      // Should show playing state
      await expect(page.locator('.sync-status, .playback-status')).toContainText(/playing|play/i, { timeout: 5000 });
    });

    test('should sync pause', async ({ page }) => {
      await page.click('button:has-text("Play"), button[aria-label*="play"]');
      await expect(page.locator('.sync-status')).toContainText(/playing/i, { timeout: 5000 });
      
      await page.click('button:has-text("Pause"), button[aria-label*="pause"], .sync-pause');
      await expect(page.locator('.sync-status')).toContainText(/paused|pause/i, { timeout: 5000 });
    });

    test('should sync seek', async ({ page }) => {
      const seekInput = page.locator('input[type="range"], input[name*="seek"], .seek-slider');
      if (await seekInput.count() > 0) {
        await seekInput.fill('120'); // 2 minutes
        await page.waitForTimeout(500);
        
        await expect(page.locator('.sync-status, .position-display')).toContainText('120', { timeout: 5000 });
      }
    });
  });

  test.describe('Video Streaming', () => {
    test.beforeEach(async ({ page }) => {
      // Login
      await page.fill('input[name="username"]', TEST_USER.username);
      await page.fill('input[name="password"]', TEST_USER.password);
      await page.click('button:has-text("Login")');
      await expect(page.locator('.main-window')).toBeVisible({ timeout: 10000 });
    });

    test('should show video player when torrent selected', async ({ page }) => {
      // Add torrent
      const magnetInput = page.locator('input[placeholder*="magnet"]');
      await magnetInput.fill('magnet:?xt=urn:btih:2222222222222222222222222222222222222222&dn=Video+Test.mp4');
      await page.click('button:has-text("Add")');
      
      await expect(page.locator('.torrent-item')).toBeVisible({ timeout: 15000 });
      
      // Select file
      await page.click('.torrent-item .file-list button:has-text("Play"), .torrent-item button:has-text("Play")');
      
      // Should show video player
      await expect(page.locator('video, .mpv-player, .video-player, [data-testid="video-player"]')).toBeVisible({ timeout: 10000 });
    });
  });
});