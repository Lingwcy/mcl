import path from 'path';
import fs from 'fs/promises';
import { mcLaunchLog } from './launch-controller';

// Store the selected Java for current launch
let mcLaunchJavaSelected: JavaEntry | null = null;

/**
 * Java entry class
 */
export class JavaEntry {
  pathJava: string;
  pathJavaw: string;
  pathFolder: string;
  versionCode: number;
  version: {
    major: number;
    minor: number;
    revision: number;
  };
  is64Bit: boolean;
  hasEnvironment: boolean;
  
  toString(): string {
    return `Java ${this.version.major}.${this.version.minor}.${this.version.revision} (${this.is64Bit ? '64' : '32'} bit) at ${this.pathFolder}`;
  }
}

/**
 * Select and prepare Java for launch
 */
export async function mcLaunchJava(task: any): Promise<void> {
  let minVer = { major: 1, minor: 0, patch: 0, revision: 0 };
  let maxVer = { major: 1, minor: 999, patch: 999, revision: 999 };
  
  // MC major version detection
  const mcVersion = mcVersionCurrent;
  
  // For 1.20.5+ (24w14a+): at least Java 21
  if ((mcVersion.releaseTime >= new Date(2024, 3, 2) && mcVersion.version.mcCodeMain === 99) || 
      (mcVersion.version.mcCodeMain > 20 && mcVersion.version.mcCodeMain !== 99) ||
      (mcVersion.version.mcCodeMain === 20 && mcVersion.version.mcCodeSub >= 5)) {
    minVer = { major: 1, minor: 21, patch: 0, revision: 0 };
  } 
  // For 1.18 pre2+: at least Java 17
  else if ((mcVersion.releaseTime >= new Date(2021, 10, 16) && mcVersion.version.mcCodeMain === 99) ||
      (mcVersion.version.mcCodeMain >= 18 && mcVersion.version.mcCodeMain !== 99)) {
    minVer = { major: 1, minor: 17, patch: 0, revision: 0 };
  }
  // For 1.17+ (21w19a+): at least Java 16
  else if ((mcVersion.releaseTime >= new Date(2021, 4, 11) && mcVersion.version.mcCodeMain === 99) ||
     (mcVersion.version.mcCodeMain >= 17 && mcVersion.version.mcCodeMain !== 99)) {
    minVer = { major: 1, minor: 16, patch: 0, revision: 0 };
  }
  // For 1.12+: at least Java 8
  else if (mcVersion.releaseTime.getFullYear() >= 2017) {
    minVer = { major: 1, minor: 8, patch: 0, revision: 0 };
  }
  // For 1.5.2-: max Java 12
  else if (mcVersion.releaseTime <= new Date(2013, 4, 1) && mcVersion.releaseTime.getFullYear() >= 2001) {
    maxVer = { major: 1, minor: 12, patch: 999, revision: 999 };
  }
  
  // Mojang recommended Java version
  if (mcVersion.jsonVersion?.java_version !== undefined) {
    const recommendedJava = parseInt(mcVersion.jsonVersion.java_version);
    mcLaunchLog(`Mojang recommends using Java ${recommendedJava}`);
    if (recommendedJava >= 22) {
      minVer = { major: 1, minor: recommendedJava, patch: 0, revision: 0 }; // Potential backward compatibility
    }
  }
  
  // OptiFine detection
  if (mcVersion.version.hasOptiFine) {
    if (mcVersion.version.mcCodeMain <= 7 && mcVersion.version.mcCodeMain > 0) {
      // <1.7: max Java 8
      maxVer = { major: 1, minor: 8, patch: 999, revision: 999 };
    } else if (mcVersion.version.mcCodeMain >= 8 && mcVersion.version.mcCodeMain <= 11) {
      // 1.8 - 1.11: must be exactly Java 8
      minVer = { major: 1, minor: 8, patch: 0, revision: 0 };
      maxVer = { major: 1, minor: 8, patch: 999, revision: 999 };
    } else if (mcVersion.version.mcCodeMain === 12) {
      // 1.12: max Java 8
      maxVer = { major: 1, minor: 8, patch: 999, revision: 999 };
    }
  }
  
  // Forge detection
  if (mcVersion.version.hasForge) {
    if (mcVersion.version.mcName === "1.7.2") {
      // 1.7.2: must use Java 7
      minVer = { major: 1, minor: 7, patch: 0, revision: 0 };
      maxVer = { major: 1, minor: 7, patch: 999, revision: 999 };
    } else if (mcVersion.version.mcCodeMain <= 12 && mcVersion.version.mcCodeMain > 0) {
      // <= 1.12: Java 8
      maxVer = { major: 1, minor: 8, patch: 999, revision: 999 };
    } else if (mcVersion.version.mcCodeMain <= 14 && mcVersion.version.mcCodeMain >= 13) {
      // 1.13 - 1.14: Java 8 - 10
      minVer = compareVersions(minVer, { major: 1, minor: 8, patch: 0, revision: 0 }) > 0 ? minVer : { major: 1, minor: 8, patch: 0, revision: 0 };
      maxVer = compareVersions(maxVer, { major: 1, minor: 10, patch: 999, revision: 999 }) < 0 ? maxVer : { major: 1, minor: 10, patch: 999, revision: 999 };
    } else if (mcVersion.version.mcCodeMain === 15) {
      // 1.15: Java 8 - 15
      minVer = compareVersions(minVer, { major: 1, minor: 8, patch: 0, revision: 0 }) > 0 ? minVer : { major: 1, minor: 8, patch: 0, revision: 0 };
      maxVer = compareVersions(maxVer, { major: 1, minor: 15, patch: 999, revision: 999 }) < 0 ? maxVer : { major: 1, minor: 15, patch: 999, revision: 999 };
    } else if (versionSortBoolean(mcVersion.version.forgeVersion, "34.0.0") && versionSortBoolean("36.2.25", mcVersion.version.forgeVersion)) {
      // 1.16, Forge 34.X ~ 36.2.25: max Java 8u320
      maxVer = compareVersions(maxVer, { major: 1, minor: 8, patch: 0, revision: 320 }) < 0 ? maxVer : { major: 1, minor: 8, patch: 0, revision: 320 };
    } else if (mcVersion.version.mcCodeMain >= 18 && mcVersion.version.mcCodeMain < 19 && mcVersion.version.hasOptiFine) {
      // 1.18: if OptiFine is installed, max Java 18
      maxVer = compareVersions(maxVer, { major: 1, minor: 18, patch: 999, revision: 999 }) < 0 ? maxVer : { major: 1, minor: 18, patch: 999, revision: 999 };
    }
  }
  
  // Fabric detection
  if (mcVersion.version.hasFabric) {
    if (mcVersion.version.mcCodeMain >= 15 && mcVersion.version.mcCodeMain <= 16 && mcVersion.version.mcCodeMain !== -1) {
      // 1.15 - 1.16: Java 8+
      minVer = compareVersions(minVer, { major: 1, minor: 8, patch: 0, revision: 0 }) > 0 ? minVer : { major: 1, minor: 8, patch: 0, revision: 0 };
    } else if (mcVersion.version.mcCodeMain >= 18 && mcVersion.version.mcCodeMain < 99) {
      // 1.18+: Java 17+
      minVer = compareVersions(minVer, { major: 1, minor: 17, patch: 0, revision: 0 }) > 0 ? minVer : { major: 1, minor: 17, patch: 0, revision: 0 };
    }
  }
  
  // Universal Pass detection
  if (getConfig("LoginType") === 2) { // McLoginType.Nide
    // At least Java 8u101
    minVer = compareVersions(minVer, { major: 1, minor: 8, patch: 0, revision: 141 }) > 0 ? minVer : { major: 1, minor: 8, patch: 0, revision: 141 };
  }
  
  // Select Java
  mcLaunchLog(`Java version requirements: minimum ${formatVersion(minVer)}, maximum ${formatVersion(maxVer)}`);
  
  // Select Java implementation would go here
  // This is a placeholder for the actual implementation
  mcLaunchJavaSelected = await javaSelect("$$", minVer, maxVer, mcVersion);
  
  if (task.isAborted) return;
  
  if (mcLaunchJavaSelected !== null) {
    mcLaunchLog(`Selected Java: ${mcLaunchJavaSelected.toString()}`);
    return;
  }
  
  // No suitable Java
  if (task.isAborted) return; // Interrupting loading will cause javaSelect to incorrectly return null
  
  mcLaunchLog("No suitable Java, need to confirm automatic download");
  
  let javaCode: string | number;
  
  // Determine which Java version to download based on requirements
  if (minVer.minor >= 22) { // Potential backward compatibility
    javaCode = minVer.minor;
    if (!await javaDownloadConfirm(`Java ${javaCode}`)) throw new Error("$$");
  } else if (minVer.minor >= 21) {
    javaCode = 21;
    if (!await javaDownloadConfirm("Java 21")) throw new Error("$$");
  } else if (minVer.minor >= 9) {
    javaCode = 17;
    if (!await javaDownloadConfirm("Java 17")) throw new Error("$$");
  } else if (maxVer.minor < 8) {
    javaCode = 7;
    if (!await javaDownloadConfirm("Java 7", true)) throw new Error("$$");
  } else if (minVer.minor > 8 || minVer.revision > 140 && maxVer.minor < 8 || maxVer.revision < 321) {
    javaCode = "8u141";
    if (!await javaDownloadConfirm("Java 8.0.141 ~ 8.0.320", true)) throw new Error("$$");
  } else if (minVer.minor > 8 || minVer.revision > 140) {
    javaCode = "8u141";
    if (!await javaDownloadConfirm("Java 8.0.141 or higher version of Java 8", true)) throw new Error("$$");
  } else if (maxVer.minor < 8 || maxVer.revision < 321) {
    javaCode = 8;
    if (!await javaDownloadConfirm("Java 8.0.320 or lower version of Java 8")) throw new Error("$$");
  } else {
    javaCode = 8;
    if (!await javaDownloadConfirm("Java 8")) throw new Error("$$");
  }
  
  // Start automatic download
  const javaLoader = javaFixLoaders(javaCode);
  
  try {
    javaLoader.start(javaCode, true);
    while (javaLoader.state === "Loading" && !task.isAborted) {
      task.progress = javaLoader.progress;
      await sleep(10);
    }
  } finally {
    javaLoader.abort(); // Ensure Java download is aborted when cancelled
  }
  
  // Check download result
  if (javaSearchLoader.state !== "Loading") javaSearchLoader.state = "Waiting";
  
  mcLaunchJavaSelected = await javaSelect("$$", minVer, maxVer, mcVersion);
  
  if (task.isAborted) return;
  
  if (mcLaunchJavaSelected !== null) {
    mcLaunchLog(`Selected Java: ${mcLaunchJavaSelected.toString()}`);
  } else {
    showHint("No available Java, launch cancelled!", "Critical");
    throw new Error("$$");
  }
}

/**
 * Extracts the Java Wrapper and returns its full file path
 */
export async function extractJavaWrapper(): Promise<string> {
  const wrapperPath = path.join(pathPure, "JavaWrapper.jar");
  console.log(`[Java] Selected Java Wrapper path: ${wrapperPath}`);
  
  try {
    await fs.writeFile(wrapperPath, getResources("JavaWrapper"));
  } catch (ex) {
    if (await fileExists(wrapperPath)) {
      // Due to unknown reasons, Java Wrapper may become read-only
      console.log("Java Wrapper file extraction failed, but file exists, will try to regenerate after deletion", ex);
      
      try {
        await fs.unlink(wrapperPath);
        await fs.writeFile(wrapperPath, getResources("JavaWrapper"));
      } catch (ex2) {
        console.log("Java Wrapper file regeneration failed, will try changing filename", ex2);
        
        const newWrapperPath = path.join(pathPure, "JavaWrapper2.jar");
        try {
          await fs.writeFile(newWrapperPath, getResources("JavaWrapper"));
          return newWrapperPath;
        } catch (ex3) {
          throw new Error(`Failed to extract Java Wrapper: ${ex3.message}`);
        }
      }
    } else {
      throw new Error(`Failed to extract Java Wrapper: ${ex.message}`);
    }
  }
  
  return wrapperPath;
}

// Helper functions
async function fileExists(path: string): Promise<boolean> {
  try {
    await fs.access(path);
    return true;
  } catch {
    return false;
  }
}

function compareVersions(ver1: any, ver2: any): number {
  if (ver1.major !== ver2.major) return ver1.major - ver2.major;
  if (ver1.minor !== ver2.minor) return ver1.minor - ver2.minor;
  if (ver1.patch !== ver2.patch) return ver1.patch - ver2.patch;
  return ver1.revision - ver2.revision;
}

function formatVersion(ver: any): string {
  return `${ver.major}.${ver.minor}.${ver.patch}.${ver.revision}`;
}

function versionSortBoolean(ver1: string, ver2: string): boolean {
  // Implementation needed
  return true;
}

// Placeholder for required functions/constants
const javaSearchLoader: any = { state: "Waiting" };
const mcVersionCurrent: any = null;
const pathPure: string = "";

// These would need actual implementations
async function javaSelect(arg1: string, minVer: any, maxVer: any, mcVersion: any): Promise<JavaEntry | null> {
  return null;
}

async function javaDownloadConfirm(javaVersion: string, legacy: boolean = false): Promise<boolean> {
  return true;
}

function javaFixLoaders(javaCode: string | number): any {
  return {
    start: (code: any, isForceRestart: boolean) => {},
    abort: () => {},
    state: "Waiting",
    progress: 0
  };
}

function getConfig(key: string): any {
  return 0;
}

function getResources(name: string): Buffer {
  return Buffer.from([]);
}

function showHint(message: string, type: string): void {
  console.log(`[${type}] ${message}`);
}

async function sleep(ms: number): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, ms));
}
