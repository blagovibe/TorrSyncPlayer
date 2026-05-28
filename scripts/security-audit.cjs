#!/usr/bin/env node
/**
 * @fileoverview Security audit script for TorrSyncPlayer dependencies.
 * 
 * Runs npm audit and provides a summary of vulnerabilities.
 * Can be run manually or integrated into CI/CD pipeline.
 * 
 * Usage:
 *   node scripts/security-audit.js
 *   npm run security:audit
 * 
 * Exit codes:
 *   0 - No vulnerabilities found
 *   1 - Vulnerabilities found (low/medium/high/critical)
 *   2 - Error running audit
 */

const { execSync } = require("node:child_process");
const path = require("node:path");
const fs = require("node:fs");

// Colors for terminal output
const colors = {
  reset: "\x1b[0m",
  red: "\x1b[31m",
  green: "\x1b[32m",
  yellow: "\x1b[33m",
  blue: "\x1b[34m",
  magenta: "\x1b[35m",
  cyan: "\x1b[36m",
  bold: "\x1b[1m",
  dim: "\x1b[2m",
};

// Severity levels with numeric values for comparison
const SEVERITY_LEVELS = {
  critical: 4,
  high: 3,
  moderate: 2,
  low: 1,
  info: 0,
};

/**
 * Format a number with color based on severity
 */
function formatSeverity(count, severity) {
  if (count === 0) return `${colors.green}0${colors.reset}`;
  
  const colorMap = {
    critical: colors.red + colors.bold,
    high: colors.red,
    moderate: colors.yellow,
    low: colors.blue,
    info: colors.dim,
  };
  
  return `${colorMap[severity] || colors.reset}${count}${colors.reset}`;
}

/**
 * Print a formatted header
 */
function printHeader(text) {
  const line = "═".repeat(text.length + 4);
  console.log(`\n${colors.cyan}${line}${colors.reset}`);
  console.log(`${colors.cyan}  ${colors.bold}${text}${colors.reset}`);
  console.log(`${colors.cyan}${line}${colors.reset}\n`);
}

/**
 * Print a section divider
 */
function printDivider() {
  console.log(`\n${colors.dim}${"─".repeat(60)}${colors.reset}\n`);
}

/**
 * Run npm audit and parse results
 */
function runNpmAudit(clientDir) {
  console.log(`${colors.blue}Running npm audit...${colors.reset}`);
  
  try {
    // Run npm audit with JSON output
    const result = execSync("npm audit --json", {
      cwd: clientDir,
      encoding: "utf-8",
      timeout: 120000, // 2 minute timeout
    });
    
    return JSON.parse(result);
  } catch (error) {
    // npm audit exits with code 1 when vulnerabilities are found
    // but still outputs valid JSON
    if (error.stdout) {
      try {
        return JSON.parse(error.stdout);
      } catch {
        // Fall through to error handling
      }
    }
    
    throw new Error(`npm audit failed: ${error.message}`);
  }
}

/**
 * Parse and categorize vulnerabilities from audit results
 */
function parseVulnerabilities(auditData) {
  const vulnerabilities = {
    critical: [],
    high: [],
    moderate: [],
    low: [],
    info: [],
  };
  
  // Handle different npm audit output formats
  const advisories = auditData.vulnerabilities || auditData.advisories || {};
  
  for (const [name, info] of Object.entries(advisories)) {
    const severity = (info.severity || "info").toLowerCase();
    const vuln = {
      name,
      severity,
      title: info.title || info.module_name || name,
      url: info.url || `https://www.npmjs.com/advisories/${info.id}`,
      via: info.via || [],
      range: info.range || "unknown",
      fixAvailable: info.fixAvailable || false,
    };
    
    if (vulnerabilities[severity]) {
      vulnerabilities[severity].push(vuln);
    } else {
      vulnerabilities.info.push(vuln);
    }
  }
  
  return vulnerabilities;
}

/**
 * Print vulnerability summary
 */
function printVulnerabilitySummary(vulnerabilities) {
  const totals = {
    critical: vulnerabilities.critical.length,
    high: vulnerabilities.high.length,
    moderate: vulnerabilities.moderate.length,
    low: vulnerabilities.low.length,
    info: vulnerabilities.info.length,
  };
  
  const total = Object.values(totals).reduce((a, b) => a + b, 0);
  
  printHeader("Security Audit Results");
  
  console.log(`${colors.bold}Vulnerability Summary:${colors.reset}\n`);
  console.log(`  ${colors.red}${colors.bold}Critical:${colors.reset}  ${formatSeverity(totals.critical, "critical")}`);
  console.log(`  ${colors.red}High:${colors.reset}      ${formatSeverity(totals.high, "high")}`);
  console.log(`  ${colors.yellow}Moderate:${colors.reset}  ${formatSeverity(totals.moderate, "moderate")}`);
  console.log(`  ${colors.blue}Low:${colors.reset}       ${formatSeverity(totals.low, "low")}`);
  console.log(`  ${colors.dim}Info:${colors.reset}      ${formatSeverity(totals.info, "info")}`);
  console.log(`\n  ${colors.bold}Total:${colors.reset}     ${total > 0 ? colors.yellow : colors.green}${total}${colors.reset}`);
  
  return { totals, total };
}

/**
 * Print detailed vulnerability information
 */
function printVulnerabilityDetails(vulnerabilities) {
  const severityOrder = ["critical", "high", "moderate", "low", "info"];
  
  for (const severity of severityOrder) {
    const vulns = vulnerabilities[severity];
    if (vulns.length === 0) continue;
    
    printDivider();
    
    const severityColors = {
      critical: colors.red + colors.bold,
      high: colors.red,
      moderate: colors.yellow,
      low: colors.blue,
      info: colors.dim,
    };
    
    console.log(`${severityColors[severity]}${severity.toUpperCase()} (${vulns.length})${colors.reset}\n`);
    
    for (const vuln of vulns) {
      console.log(`  ${colors.bold}${vuln.name}${colors.reset}`);
      console.log(`  ${colors.dim}Severity:${colors.reset} ${vuln.severity}`);
      console.log(`  ${colors.dim}Range:${colors.reset} ${vuln.range}`);
      
      if (vuln.fixAvailable) {
        console.log(`  ${colors.green}✓ Fix available${colors.reset}`);
      }
      
      if (vuln.url) {
        console.log(`  ${colors.dim}URL:${colors.reset} ${vuln.url}`);
      }
      
      console.log();
    }
  }
}

/**
 * Check for known problematic packages
 */
function checkKnownProblematicPackages(clientDir) {
  console.log(`${colors.blue}Checking for known problematic packages...${colors.reset}`);
  
  const packageJsonPath = path.join(clientDir, "package.json");
  
  if (!fs.existsSync(packageJsonPath)) {
    console.log(`${colors.yellow}Warning: package.json not found at ${packageJsonPath}${colors.reset}`);
    return [];
  }
  
  const packageJson = JSON.parse(fs.readFileSync(packageJsonPath, "utf-8"));
  const allDeps = {
    ...packageJson.dependencies,
    ...packageJson.devDependencies,
  };
  
  // Packages with known security concerns
  const problematicPatterns = [
    { pattern: /eval/i, reason: "Uses eval() - potential code injection risk" },
    { pattern: /exec-sync/i, reason: "Synchronous execution - potential DoS risk" },
  ];
  
  const warnings = [];
  
  for (const [pkg, version] of Object.entries(allDeps)) {
    for (const { pattern, reason } of problematicPatterns) {
      if (pattern.test(pkg)) {
        warnings.push({ package: pkg, version, reason });
      }
    }
  }
  
  if (warnings.length > 0) {
    printDivider();
    console.log(`${colors.yellow}${colors.bold}Package Warnings:${colors.reset}\n`);
    
    for (const warning of warnings) {
      console.log(`  ${colors.yellow}⚠${colors.reset} ${colors.bold}${warning.package}@${warning.version}${colors.reset}`);
      console.log(`    ${colors.dim}${warning.reason}${colors.reset}`);
    }
  } else {
    console.log(`${colors.green}✓ No problematic package patterns found${colors.reset}`);
  }
  
  return warnings;
}

/**
 * Generate recommendations based on findings
 */
function generateRecommendations(vulnerabilities, totals) {
  printDivider();
  console.log(`${colors.bold}Recommendations:${colors.reset}\n`);
  
  if (totals.total === 0) {
    console.log(`  ${colors.green}✓ No vulnerabilities found! Your dependencies are secure.${colors.reset}`);
    return;
  }
  
  if (totals.critical > 0) {
    console.log(`  ${colors.red}${colors.bold}1. URGENT: Update critical vulnerabilities immediately${colors.reset}`);
    console.log(`     Run: npm audit fix`);
    console.log(`     If that doesn't work, manually update affected packages\n`);
  }
  
  if (totals.high > 0) {
    console.log(`  ${colors.red}2. HIGH: Address high severity issues soon${colors.reset}`);
    console.log(`     Review each vulnerability and apply fixes\n`);
  }
  
  if (totals.moderate > 0) {
    console.log(`  ${colors.yellow}3. MODERATE: Plan updates for moderate issues${colors.reset}`);
    console.log(`     Include in next maintenance window\n`);
  }
  
  console.log(`  ${colors.blue}4. General best practices:${colors.reset}`);
  console.log(`     - Run 'npm audit fix' to auto-fix what's possible`);
  console.log(`     - Review 'npm audit' output for manual fixes`);
  console.log(`     - Keep dependencies updated regularly`);
  console.log(`     - Use 'npm outdated' to check for updates`);
}

/**
 * Main audit function
 */
async function main() {
  const startTime = Date.now();
  
  printHeader("TorrSyncPlayer Security Audit");
  
  console.log(`${colors.dim}Started at: ${new Date().toISOString()}${colors.reset}`);
  
  // Find client directory
  const scriptDir = __dirname;
  const rootDir = path.resolve(scriptDir, "..");
  const clientDir = path.join(rootDir, "client");
  
  if (!fs.existsSync(clientDir)) {
    console.error(`${colors.red}Error: Client directory not found at ${clientDir}${colors.reset}`);
    process.exit(2);
  }
  
  console.log(`${colors.dim}Client directory: ${clientDir}${colors.reset}`);
  
  // Run npm audit
  let auditData;
  try {
    auditData = runNpmAudit(clientDir);
  } catch (error) {
    console.error(`${colors.red}Error running npm audit: ${error.message}${colors.reset}`);
    process.exit(2);
  }
  
  // Parse and display results
  const vulnerabilities = parseVulnerabilities(auditData);
  const { totals, total } = printVulnerabilitySummary(vulnerabilities);
  
  // Show details if vulnerabilities found
  if (total > 0) {
    printVulnerabilityDetails(vulnerabilities);
  }
  
  // Check for problematic packages
  checkKnownProblematicPackages(clientDir);
  
  // Generate recommendations
  generateRecommendations(vulnerabilities, totals);
  
  // Print summary
  const duration = ((Date.now() - startTime) / 1000).toFixed(2);
  printDivider();
  console.log(`${colors.dim}Audit completed in ${duration}s${colors.reset}`);
  
  if (total === 0) {
    console.log(`${colors.green}${colors.bold}✓ Security audit passed!${colors.reset}`);
  } else {
    console.log(`${colors.yellow}${colors.bold}⚠ Security audit found ${total} vulnerability(ies)${colors.reset}`);
  }
  
  // Exit with appropriate code
  // 0 = no vulnerabilities, 1 = vulnerabilities found
  process.exit(total > 0 ? 1 : 0);
}

// Run main function
main().catch((error) => {
  console.error(`${colors.red}Unexpected error: ${error.message}${colors.reset}`);
  console.error(error.stack);
  process.exit(2);
});
