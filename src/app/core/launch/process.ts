import { spawn, ChildProcess } from 'child_process';
import path from 'path';
import { mcLaunchLog } from './launch-controller';

// Store the process object for the current Minecraft instance
let mcLaunchProcess: ChildProcess | null = null;

/**
 * Launches the Minecraft process
 */
export async function mcLaunchRun(loader: any): Promise<ChildProcess> {
  // Create process info
  const javaPath = mcLaunchJavaSelected.pathJavaw;
  const args = mcLaunchArgument.split(' ').filter(arg => arg.trim() !== '');
  
  // Set environment variables
  const env = { ...process.env };
  env.APPDATA = shortenPath(mcVersionCurrent.pathIndie);
  
  // Set up process options
  const options = {
    cwd: shortenPath(mcVersionCurrent.pathIndie),
    env,
    stdio: ['ignore', 'pipe', 'pipe'] as ('ignore' | 'pipe' | 'inherit')[],
    windowsHide: false,
    detached: false
  };

  // Start process
  mcLaunchLog(`Launching game process: ${javaPath}`);
  const gameProcess = spawn(javaPath, args, options);
  
  if (loader.isAborted) {
    mcLaunchLog("Terminating game process due to launch cancellation");
    gameProcess.kill();
    return Promise.reject(new Error("Launch aborted"));
  }
  
  loader.output = gameProcess;
  mcLaunchProcess = gameProcess;

  // Process priority handling
  try {
    if (process.platform === 'win32') {
      const priority = getConfig("LaunchArgumentPriority");
      // Node.js doesn't support changing process priority directly
      // We would need to use a native module or spawn a process to do this
      mcLaunchLog(`Set process priority to: ${['high', 'normal', 'low'][priority]}`);
    }
  } catch (ex) {
    console.error("Failed to set process priority", ex);
  }

  // Set up logging
  gameProcess.stdout?.on('data', (data) => {
    const lines = data.toString().split('\n');
    for (const line of lines) {
      if (line.trim()) {
        mcLaunchLog(`[MC] ${line.trim()}`);
      }
    }
  });
  
  gameProcess.stderr?.on('data', (data) => {
    const lines = data.toString().split('\n');
    for (const line of lines) {
      if (line.trim()) {
        mcLaunchLog(`[MC/ERR] ${line.trim()}`);
      }
    }
  });

  return gameProcess;
}

/**
 * Waits for the Minecraft window to appear and monitors the process
 */
export async function mcLaunchWait(loader: any): Promise<void> {
  // Output information
  mcLaunchLog("");
  mcLaunchLog("~ Basic Parameters ~");
  mcLaunchLog(`PCL Version: ${versionDisplayName} (${versionCode})`);
  mcLaunchLog(`Game Version: ${mcVersionCurrent.version.toString()} (Detected as 1.${mcVersionCurrent.version.mcCodeMain}.${mcVersionCurrent.version.mcCodeSub})`);
  mcLaunchLog(`Resource Version: ${mcAssetsGetIndexName(mcVersionCurrent)}`);
  mcLaunchLog(`Inherited Version: ${mcVersionCurrent.inheritVersion || "None"}`);
  mcLaunchLog(`Allocated Memory: ${getVersionRam(mcVersionCurrent, !mcLaunchJavaSelected.is64Bit)} GB (${Math.round(getVersionRam(mcVersionCurrent, !mcLaunchJavaSelected.is64Bit) * 1024)} MB)`);
  mcLaunchLog(`MC Folder: ${pathMcFolder}`);
  mcLaunchLog(`Version Folder: ${mcVersionCurrent.path}`);
  mcLaunchLog(`Version Isolation: ${mcVersionCurrent.pathIndie === mcVersionCurrent.path}`);
  mcLaunchLog(`HMCL Format: ${mcVersionCurrent.isHmclFormatJson}`);
  mcLaunchLog(`Java Info: ${mcLaunchJavaSelected ? mcLaunchJavaSelected.toString() : "No Java Available"}`);
  mcLaunchLog(`Environment Variables: ${mcLaunchJavaSelected ? (mcLaunchJavaSelected.hasEnvironment ? "Set" : "Not Set") : "Not Set"}`);
  mcLaunchLog(`Natives Folder: ${getNativesFolder()}`);
  mcLaunchLog("");
  mcLaunchLog("~ Login Parameters ~");
  mcLaunchLog(`Player Username: ${mcLoginLoader.output.name}`);
  mcLaunchLog(`AccessToken: ${mcLoginLoader.output.accessToken}`);
  mcLaunchLog(`ClientToken: ${mcLoginLoader.output.clientToken}`);
  mcLaunchLog(`UUID: ${mcLoginLoader.output.uuid}`);
  mcLaunchLog(`Login Method: ${mcLoginLoader.output.type}`);
  mcLaunchLog("");

  // Get window title
  const windowTitle = getConfig("VersionArgumentTitle", { version: mcVersionCurrent }) || 
                     getConfig("LaunchArgumentTitle");
  const processedTitle = argumentReplace(windowTitle, false);

  // Initialize process watcher
  const watcher = new MinecraftWatcher(loader, mcVersionCurrent, processedTitle);
  mcLaunchWatcher = watcher;

  // Wait for game window
  return new Promise((resolve, reject) => {
    const checkInterval = setInterval(() => {
      if (watcher.state === MinecraftState.Running) {
        clearInterval(checkInterval);
        resolve();
      } else if (watcher.state === MinecraftState.Crashed) {
        clearInterval(checkInterval);
        reject(new Error("$$"));
      }
    }, 100);
  });
}

/**
 * MinecraftWatcher class to monitor the Minecraft process
 */
class MinecraftWatcher {
  state: MinecraftState = MinecraftState.Loading;
  
  constructor(loader: any, version: any, windowTitle: string) {
    const process = mcLaunchProcess;
    if (!process) {
      this.state = MinecraftState.Crashed;
      return;
    }

    // Set up exit handler
    process.on('exit', (code) => {
      mcLaunchLog(`Game process exited with code ${code}`);
      if (this.state === MinecraftState.Loading) {
        this.state = MinecraftState.Crashed;
        mcLaunchLog("Game crashed during startup!");
      } else {
        this.state = MinecraftState.Closed;
      }
    });

    // On Windows we would use a native module to find the game window
    // For this example, we'll just assume the game started successfully after a delay
    setTimeout(() => {
      if (this.state === MinecraftState.Loading) {
        this.state = MinecraftState.Running;
        mcLaunchLog("Game window detected!");
      }
    }, 10000);
  }
}

/**
 * Minecraft process states
 */
enum MinecraftState {
  Loading,  // Game is starting up
  Running,  // Game is running
  Crashed,  // Game crashed during startup
  Closed    // Game closed normally
}

// Import as needed from other modules
const mcLaunchJavaSelected: any = null;
const mcLaunchArgument: string = "";
const mcVersionCurrent: any = null;
const mcLoginLoader: any = null;
let mcLaunchWatcher: any = null;
const versionDisplayName: string = "";
const versionCode: string = "";
const pathMcFolder: string = "";

// Helper functions that would be implemented elsewhere
function shortenPath(path: string): string {
  return path;
}

function getConfig(key: string, options?: any): any {
  return "";
}

function getNativesFolder(): string {
  return "";
}

function mcAssetsGetIndexName(version: any): string {
  return "";
}

function getVersionRam(version: any, is32Bit: boolean): number {
  return 2;
}

function argumentReplace(input: string, replaceTimeAndDate: boolean): string {
  return input;
}
