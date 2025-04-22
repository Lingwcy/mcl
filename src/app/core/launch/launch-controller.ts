import { McLaunchOptions } from './types';
import { getMcLaunchArguments, argumentReplace, saveLaunchScript } from './arguments';
import { shortenPath, runInUiWait, sleep } from './utils';
import { 
  mcVersionCurrent as configMcVersionCurrent,
  currentLaunchOptions as configCurrentLaunchOptions,
  setConfig
} from './config';

// Global state - change const to let for mutable variables
let currentLaunchOptions: McLaunchOptions | null = null;
let mcLaunchLoaderReal: any = null; // Replace with proper type when implemented
let mcLaunchProcess: any = null; // Will be Node.js Process or equivalent
let mcLaunchWatcher: any = null; // Replace with proper type when implemented
let abortHint: string | null = null;
// Variable for mcVersionCurrent should be let, not const
let mcVersionCurrent: any = configMcVersionCurrent;

/**
 * Logs launch information
 * @param text The text to log
 */
export function mcLaunchLog(text: string): void {
  // Filter secrets
  text = secretFilter(text, "*");
  
  // Update UI with log message
  console.log(`[${getCurrentTime()}] ${text}`);
  
  // Log to system log
  console.log(`[Launch] ${text}`);
}

/**
 * Attempts to launch Minecraft. Must be called from UI thread.
 * Returns whether launch actually started (if not, an error message was shown).
 * @param options Optional launch options
 */
export async function mcLaunchStart(options: McLaunchOptions | null = null): Promise<boolean> {
  currentLaunchOptions = options || new McLaunchOptions();
  configCurrentLaunchOptions.serverIp = currentLaunchOptions.serverIp;
  configCurrentLaunchOptions.saveBatch = currentLaunchOptions.saveBatch;
  configCurrentLaunchOptions.version = currentLaunchOptions.version;
  configCurrentLaunchOptions.extraArgs = currentLaunchOptions.extraArgs;
  
  // Pre-checks
  if (mcLaunchLoader.state === 'Loading') {
    showHint("A game is already launching!", "Critical");
    return false;
  }
  
  // Force switch to required launch version
  if (currentLaunchOptions.version && mcVersionCurrent !== currentLaunchOptions.version) {
    mcLaunchLog(`Switching to version ${currentLaunchOptions.version.name} before launch`);
    
    // Check version
    currentLaunchOptions.version.load();
    if (currentLaunchOptions.version.state === "Error") {
      showHint(`Cannot launch Minecraft: ${currentLaunchOptions.version.info}`, "Critical");
      return false;
    }
    
    // Switch version
    mcVersionCurrent = currentLaunchOptions.version;
    setConfig("LaunchVersionSelect", mcVersionCurrent.name);
    refreshButtonsUI();
    refreshPage(false, false);
  }
  
  // Prevent entering version selection page
  preventVersionSelectPage();
  
  // Actually start the loader
  mcLaunchLoader.start(options, true);
  return true;
}

/**
 * Main launch method implementation
 */
async function mcLaunchStartImpl(loader: any) {
  // Start animation
  await runInUiWait(() => pageChangeToLaunching());
  
  // Pre-checks (pre-check errors will be thrown directly)
  try {
    await mcLaunchPrecheck();
    mcLaunchLog("Pre-checks passed");
  } catch (ex: any) {
    if (!ex.message.startsWith("$$")) {
      showHint(ex.message, "Critical");
    }
    throw ex;
  }
  
  // Formal loading
  try {
    // Construct main loader
    const loaders = [
      // Would implement loaders similar to the VB code
      // For now, listing the main steps
      createLoader("Get Java", mcLaunchJava, { progressWeight: 4, block: false }),
      mcLoginLoader,
      createLoader("Complete files", dlClientFix(), { progressWeight: 15, show: false }),
      createLoader("Get launch parameters", mcLaunchArgumentMain, { progressWeight: 2 }),
      createLoader("Extract files", mcLaunchNatives, { progressWeight: 2 }),
      createLoader("Pre-launch processing", mcLaunchPrerun, { progressWeight: 1 }),
      createLoader("Execute custom commands", mcLaunchCustom, { progressWeight: 1 }),
      createLoader("Launch process", mcLaunchRun, { progressWeight: 2 }),
      createLoader("Wait for game window", mcLaunchWait, { progressWeight: 1 }),
      createLoader("End processing", mcLaunchEnd, { progressWeight: 1 })
    ];
    
    // Memory optimization
    const versionRamOptimize = getConfig("VersionRamOptimize", mcVersionCurrent);
    if (
      (versionRamOptimize === 0 && getConfig("LaunchArgumentRam")) ||
      versionRamOptimize === 1
    ) {
      loaders[2].block = false;
      loaders.splice(3, 0, createLoader("Memory optimization", mcLaunchMemoryOptimize, { progressWeight: 30 }));
    }
    
    const launchLoader = createComboLoader("Minecraft Launch", loaders, { show: false });
    
    // Reset login loader if finished
    if (mcLoginLoader.state === "Finished") {
      mcLoginLoader.state = "Waiting";
    }
    
    // Wait for loader to execute and update UI
    mcLaunchLoaderReal = launchLoader;
    abortHint = null;
    launchLoader.start();
    
    // Check save batch option
    if (currentLaunchOptions?.saveBatch) {
      const scriptPath = await saveLaunchScript(currentLaunchOptions.saveBatch);
      mcLaunchLog(`Launch script saved to: ${scriptPath}`);
      abortHint = "Launch script export successful!";
      openExplorer(scriptPath);
      launchLoader.abort();
      return;
    }
    
    // Taskbar progress bar
    updateTaskbar(launchLoader);
    
    // Monitor progress until complete
    while (launchLoader.state === "Loading") {
      refreshLaunchingUI();
      await sleep(200);
    }
    
    refreshLaunchingUI();
    
    // Handle success or failure
    switch (launchLoader.state) {
      case "Finished":
        showHint(`${mcVersionCurrent.name} launched successfully!`, "Finish");
        break;
      case "Aborted":
        if (abortHint === null) {
          showHint(
            currentLaunchOptions?.saveBatch === null ? "Launch cancelled!" : "Export launch script cancelled!",
            "Info"
          );
        } else {
          showHint(abortHint, "Finish");
        }
        break;
      case "Failed":
        throw launchLoader.error;
      default:
        throw new Error(`Invalid state change: ${launchLoader.state}`);
    }
    
  } catch (ex: any) {
    let currentEx = ex;
    
    while (true) {
      if (currentEx.message.startsWith("$")) {
        // If there's an error message starting with $, use it as the display message
        // If the error message is $$, don't show a hint
        if (currentEx.message !== "$$") {
          showMessageBox(
            currentEx.message.substring(1),
            currentLaunchOptions?.saveBatch === null ? "Launch Failed" : "Export Launch Script Failed"
          );
        }
        throw currentEx;
      } else if (currentEx.innerException) {
        // Check next level error
        currentEx = currentEx.innerException;
        continue;
      } else {
        // No specially processed error message
        mcLaunchLog(`Error: ${getExceptionDetail(ex)}`);
        logError(
          ex,
          currentLaunchOptions?.saveBatch === null ? "Minecraft launch failed" : "Export launch script failed",
          "Msgbox",
          currentLaunchOptions?.saveBatch === null ? "Launch failed" : "Export launch script failed"
        );
        throw ex;
      }
    }
  }
}

// Helper function to filter sensitive information from logs
export function secretFilter(text: string, replacement: string): string {
  if (!text) return text;
  
  // Replace access tokens, client tokens, and other sensitive info with replacement characters
  return text
    .replace(/accessToken[":]+"([^"]+)"/g, `accessToken":"${replacement.repeat(10)}"`)
    .replace(/clientToken[":]+"([^"]+)"/g, `clientToken":"${replacement.repeat(10)}"`)
    .replace(/-Dauthlibinjector\.yggdrasil\.prefetched=([^\s]+)/g, `-Dauthlibinjector.yggdrasil.prefetched=${replacement.repeat(5)}`);
}

// Helper function to get current time string
function getCurrentTime(): string {
  return new Date().toLocaleTimeString();
}

// Placeholder for required functions/constants from VB that would need to be implemented
const mcLaunchLoader: any = { state: "Waiting" };
const mcLoginLoader: any = { state: "Waiting" };

// Helper function stubs that would need to be implemented
function showHint(message: string, type: string): void {
  console.log(`[${type}] ${message}`);
}

function showMessageBox(message: string, title: string): void {
  console.log(`[${title}] ${message}`);
}

function refreshButtonsUI(): void {
  // Implementation needed
}

function refreshPage(arg1: boolean, arg2: boolean): void {
  // Implementation needed
}

function preventVersionSelectPage(): void {
  // Implementation needed
}

function pageChangeToLaunching(): void {
  // Implementation needed
}

function getConfig(key: string, version?: any): any {
  return null; // Implementation needed
}

function openExplorer(path: string): void {
  // Implementation needed
}

function getExceptionDetail(ex: any): string {
  return ex.message; // Implementation needed
}

function logError(ex: any, message: string, level: string, title: string): void {
  console.error(`[${level}] ${title}: ${message}`, ex);
}

// These functions would be imported from other modules
function mcLaunchJava(): Promise<void> {
  return Promise.resolve();
}

function mcLaunchArgumentMain(): Promise<any> {
  return Promise.resolve([]);
}

function mcLaunchNatives(): Promise<void> {
  return Promise.resolve();
}

function mcLaunchPrerun(): Promise<void> {
  return Promise.resolve();
}

function mcLaunchCustom(): Promise<void> {
  return Promise.resolve();
}

function mcLaunchRun(): Promise<void> {
  return Promise.resolve();
}

function mcLaunchWait(): Promise<void> {
  return Promise.resolve();
}

function mcLaunchEnd(): Promise<void> {
  return Promise.resolve();
}

function mcLaunchMemoryOptimize(): Promise<void> {
  return Promise.resolve();
}

function mcLaunchPrecheck(): Promise<void> {
  return Promise.resolve();
}

function dlClientFix(): any {
  return {};
}

// Helper functions that were missing
function createLoader(name: string, func: Function, options?: any): any {
  return { name, func, ...options };
}

function createComboLoader(name: string, loaders: any[], options?: any): any {
  return { name, loaders, ...options };
}

function updateTaskbar(loader: any): void {
  // Placeholder implementation for updateTaskbar
  console.log(`Updating taskbar progress: ${loader.progress * 100}%`);
}

function refreshLaunchingUI(): void {
  // Placeholder implementation for refreshLaunchingUI
  console.log("Refreshing launching UI...");
}
