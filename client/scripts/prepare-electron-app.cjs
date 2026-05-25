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
if (fs.existsSync(path.join(projectDir, "local-packages"))) {
  fs.cpSync(path.join(projectDir, "local-packages"), path.join(stageDir, "local-packages"), { recursive: true });
}

const stagedPackage = {
  ...sourcePackage,
  private: true,
  main: "electron/main.cjs",
  dependencies: {
    ...sourcePackage.dependencies,
    ip: "file:./local-packages/ip-patched",
  },
  overrides: sourcePackage.overrides,
  build: {
    appId: "com.torrsyncplayer.app",
    productName: "TorrSyncPlayer",
    icon: path.join(projectDir, "..", "TorrSyncPlayer_Icon.ico"),
    electronVersion,
    directories: {
      output: path.join(projectDir, "release"),
    },
    files: ["**/*"],
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

execFileSync(process.execPath, [npmCli, "install", "--omit=dev", "--no-audit", "--fund=false"], {
  cwd: stageDir,
  stdio: "inherit",
});

const patchedIpDir = path.join(stageDir, "local-packages", "ip-patched");
const patchedPackageJson = JSON.parse(fs.readFileSync(path.join(patchedIpDir, "package.json"), "utf8"));
patchedPackageJson.name = "ip";
fs.writeFileSync(path.join(patchedIpDir, "package.json"), `${JSON.stringify(patchedPackageJson, null, 2)}\n`);

function ensureIpModule(nodeModulesDir) {
  const ipPath = path.join(nodeModulesDir, "ip");
  let needsFix = false;
  if (fs.existsSync(ipPath)) {
    try {
      const contents = fs.readdirSync(ipPath);
      if (contents.length === 0) needsFix = true;
    } catch {
      needsFix = true;
    }
    if (needsFix) fs.rmSync(ipPath, { recursive: true, force: true });
  } else {
    needsFix = true;
  }
  if (needsFix) {
    const parentPkgPath = path.join(nodeModulesDir, "..", "package.json");
    try {
      const parentPkg = JSON.parse(fs.readFileSync(parentPkgPath, "utf8"));
      const deps = parentPkg.dependencies || {};
      const overrides = parentPkg.overrides || {};
      if ("ip" in deps || "ip" in overrides) {
        fs.cpSync(patchedIpDir, ipPath, { recursive: true });
      }
    } catch {}
  }
}

function walkForNodeModules(dir) {
  let entries;
  try {
    entries = fs.readdirSync(dir, { withFileTypes: true });
  } catch {
    return;
  }
  for (const entry of entries) {
    if (!entry.isDirectory()) continue;
    const fullPath = path.join(dir, entry.name);
    if (entry.name === "node_modules") {
      ensureIpModule(fullPath);
    }
    walkForNodeModules(fullPath);
  }
}

walkForNodeModules(stageDir);
