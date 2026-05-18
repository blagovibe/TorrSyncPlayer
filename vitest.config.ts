import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    include: ["client/src/**/__tests__/**/*.{test,spec}.{js,ts,tsx}", "client/src/**/*.{test,spec}.{js,ts,tsx}"],
    exclude: ["node_modules", "dist", ".electron-app", "client/electron"],
  },
});
