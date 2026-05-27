const { execSync } = require("child_process");
const fs = require("fs");
const path = require("path");

const cacheDir = path.join(process.env.LOCALAPPDATA, "electron-builder", "Cache", "winCodeSign");

// Find all .7z files in cache
const archives = fs.readdirSync(cacheDir).filter(f => f.endsWith(".7z"));

for (const archive of archives) {
  const hash = archive.replace(".7z", "");
  const destDir = path.join(cacheDir, hash);
  
  if (fs.existsSync(destDir) && fs.readdirSync(destDir).length > 0) {
    console.log(`Skipping ${hash} - already extracted`);
    continue;
  }
  
  fs.mkdirSync(destDir, { recursive: true });
  
  console.log(`Extracting ${archive}...`);

  // Extract using 7za with -snld- (don't follow symlinks, skip them)
  // But we need the actual -snl flag behavior. Let's try with -- to override
  let sevenZipPath;
  try {
    const sevenZipPkg = require.resolve("7zip-bin");
    const sevenZipDir = path.dirname(sevenZipPkg);
    sevenZipPath = path.join(sevenZipDir, "..", "..", "win", "x64", "7za.exe");
    if (!fs.existsSync(sevenZipPath)) {
      sevenZipPath = require.resolve("7zip-bin/win/x64/7za.exe");
    }
  } catch {
    const electronBuilderDir = path.join(__dirname, "..", "client", "node_modules", "electron-builder");
    sevenZipPath = path.join(electronBuilderDir, "node_modules", "7zip-bin", "win", "x64", "7za.exe");
  }
  
  try {
    // First attempt: extract with -snl- to skip symlink creation
    execSync(`"${sevenZipPath}" x -snl- -bd "${path.join(cacheDir, archive)}" "-o${destDir}" -aoa`, {
      stdio: "pipe"
    });
    console.log(`  Extracted successfully (symlinks skipped)`);
  } catch (err) {
    // If that doesn't work, try extracting everything except darwin folder
    console.log(`  Failed with snl-, trying selective extraction...`);
    try {
      execSync(`"${sevenZipPath}" x -bd "${path.join(cacheDir, archive)}" "-o${destDir}" -aoa -x!darwin`, {
        stdio: "pipe"
      });
      console.log(`  Extracted successfully (darwin excluded)`);
    } catch (err2) {
      console.log(`  Failed: ${err2.message}`);
    }
  }
}
