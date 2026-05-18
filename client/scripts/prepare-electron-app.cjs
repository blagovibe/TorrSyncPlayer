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
fs.cpSync(path.join(projectDir, "package-lock.json"), path.join(stageDir, "package-lock.json"));

const stagedPackage = {
  ...sourcePackage,
  private: true,
  main: "electron/main.cjs",
  build: {
    appId: "com.torrsyncplayer.app",
    productName: "TorrSyncPlayer",
    electronVersion,
    directories: {
      output: "../release",
    },
    files: ["**/*"],
    asar: true,
    npmRebuild: false,
    nodeGypRebuild: false,
    artifactName: "${productName}-${version}-${os}-${arch}.${ext}",
    linux: {
      target: ["AppImage"],
      category: "Video",
    },
    win: {
      target: ["nsis", "portable"],
    },
    nsis: {
      oneClick: false,
      allowToChangeInstallationDirectory: true,
      perMachine: false,
    },
  },
};

fs.writeFileSync(path.join(stageDir, "package.json"), `${JSON.stringify(stagedPackage, null, 2)}\n`);

const npmCli = process.env.npm_execpath;
if (!npmCli) {
  throw new Error("npm_execpath is not set. Run this script through npm run electron:build.");
}

execFileSync(process.execPath, [npmCli, "ci", "--omit=dev", "--no-audit", "--fund=false"], {
  cwd: stageDir,
  stdio: "inherit",
});
