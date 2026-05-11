const fs = require("node:fs");
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
fs.mkdirSync(path.join(stageDir, "node_modules"));
fs.writeFileSync(
  path.join(stageDir, "electron-builder.before-build.cjs"),
  "exports.default = async function beforeBuild() { return false; };\n",
);

const stagedPackage = {
  name: sourcePackage.name,
  version: sourcePackage.version,
  description: sourcePackage.description,
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
    beforeBuild: "electron-builder.before-build.cjs",
    artifactName: "${productName}-${version}-${os}-${arch}.${ext}",
    linux: {
      target: ["AppImage"],
      category: "Video",
      icon: "../src-tauri/icons/icon.png",
    },
    win: {
      target: ["nsis", "portable"],
      icon: "../src-tauri/icons/icon.ico",
    },
    nsis: {
      oneClick: false,
      allowToChangeInstallationDirectory: true,
      perMachine: false,
    },
  },
};

fs.writeFileSync(path.join(stageDir, "package.json"), `${JSON.stringify(stagedPackage, null, 2)}\n`);
