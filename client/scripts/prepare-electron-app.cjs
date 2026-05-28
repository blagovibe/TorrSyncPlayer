const fs = require("node:fs");
const { execFileSync } = require("node:child_process");
const path = require("node:path");

const projectDir = path.resolve(__dirname, "..");
const stageDir = path.join(projectDir, ".electron-app");
const sourcePackage = require(path.join(projectDir, "package.json"));
const electronVersion = sourcePackage.devDependencies.electron.replace(/^[^\d]*/, "");

for (const requiredPath of ["dist/index.html", "electron/main.cjs"]) {
  const absolutePath = path.join(projectDir, requiredPath);
  if (!fs.existsSync(absolutePath)) {
    throw new Error(`Missing ${requiredPath}. Run npm run build before packaging Electron.`);
  }
}

fs.rmSync(stageDir, { recursive: true, force: true });
fs.mkdirSync(stageDir, { recursive: true });
fs.cpSync(path.join(projectDir, "dist"), path.join(stageDir, "dist"), { recursive: true });
fs.cpSync(path.join(projectDir, "electron"), path.join(stageDir, "electron"), { recursive: true });
fs.cpSync(path.join(projectDir, "torrent-shared.json"), path.join(stageDir, "torrent-shared.json"));

const sharedConfig = JSON.parse(fs.readFileSync(path.join(projectDir, "torrent-shared.json"), "utf8"));
const preloadPath = path.join(stageDir, "electron", "preload.cjs");
const originalPreload = fs.readFileSync(preloadPath, "utf8");
const injectedPreload = originalPreload.replace(
  'const shared = require("../torrent-shared.json");',
  `const shared = ${JSON.stringify(sharedConfig)};`
);
const maxTorrentBytesLine = `const MAX_TORRENT_FILE_BYTES = ${sharedConfig.maxTorrentFileBytes};`;
const injectedWithConstants = injectedPreload.replace(
  "const MAX_TORRENT_FILE_BYTES = shared.maxTorrentFileBytes;",
  maxTorrentBytesLine
);
fs.writeFileSync(preloadPath, injectedWithConstants);

for (const cjsFile of ["electron/torrent-constants.cjs", "electron/torrent-bridge.cjs", "electron/audio-session-manager.cjs"]) {
  const cjsPath = path.join(stageDir, cjsFile);
  if (fs.existsSync(cjsPath)) {
    const original = fs.readFileSync(cjsPath, "utf8");
    const injected = original.replace(
      'require("../torrent-shared.json")',
      `(${JSON.stringify(sharedConfig)})`
    );
    if (injected !== original) {
      fs.writeFileSync(cjsPath, injected);
    }
  }
}

const stagedPackage = {
  ...sourcePackage,
  author: "TorrSyncPlayer",
  private: true,
  main: "electron/main.cjs",
  overrides: sourcePackage.overrides,
  build: {
    appId: "com.torrsyncplayer.app",
    productName: "TorrSyncPlayer",
    icon: path.join(projectDir, "..", "TorrSyncPlayer_Icon.ico"),
    electronVersion,
    directories: {
      output: "release",
    },
    files: ["**/*", "!**/*.map", "!**/__tests__/**", "!**/test/**", "!**/tests/**", "!**/*.test.*", "!**/*.spec.*", "!**/docs/**", "!**/patches/**"],
    asar: true,
    npmRebuild: false,
    nodeGypRebuild: false,
    forceCodeSigning: false,
    artifactName: "${productName}-${version}-${os}-${arch}.${ext}",
    linux: {
      target: ["AppImage"],
      category: "Video",
    },
    win: {
      target: ["portable"],
      icon: path.join(projectDir, "..", "TorrSyncPlayer_Icon.ico"),
    },
    mac: {
      icon: path.join(projectDir, "..", "TorrSyncPlayer_Icon.icns"),
    },
  },
};

fs.writeFileSync(path.join(stageDir, "package.json"), `${JSON.stringify(stagedPackage, null, 2)}\n`);

const npmCli = process.env.npm_execpath;
if (!npmCli) {
  throw new Error("npm_execpath is not set. Run this script through npm run electron:build.");
}

try {
  execFileSync(process.execPath, [npmCli, "install", "--omit=dev", "--no-audit", "--fund=false"], {
    cwd: stageDir,
    stdio: "inherit",
  });
} catch (error) {
  throw new Error(`npm install failed in ${stageDir}: ${error?.message ?? error}`);
}

// npm overrides field in package.json handles top-level ip package resolution.
// Nested node_modules instances are addressed via the override mechanism.
