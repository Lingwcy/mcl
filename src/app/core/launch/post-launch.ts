import { getConfig, setConfig, mcVersionCurrent } from './config';
import { mcLaunchLog } from './launch-controller';
import { runInUi } from './utils';

// Maintain local references to the process and watcher
let mcLaunchProcess: any = null;
let mcLaunchWatcher: any = null;

/**
 * Performs actions after Minecraft has been launched successfully
 */
export async function mcLaunchEnd(): Promise<void> {
  mcLaunchLog("Starting post-launch processing");

  // Pause or resume music playback
  if (getConfig("UiMusicStop")) {
    runInUi(() => {
      if (musicPause()) {
        console.log("[Music] Music playback paused according to settings");
      }
    });
  } else if (getConfig("UiMusicStart")) {
    runInUi(() => {
      if (musicResume()) {
        console.log("[Music] Music playback resumed according to settings");
      }
    });
  }

  // Handle launcher visibility
  mcLaunchLog("Launcher visibility: " + getConfig("LaunchArgumentVisible"));
  switch (getConfig("LaunchArgumentVisible")) {
    case 0:
      // Close directly
      mcLaunchLog("Closing launcher as configured");
      runInUi(() => endProgram(false));
      break;
    case 2:
    case 3:
      // Hide
      mcLaunchLog("Hiding launcher as configured");
      runInUi(() => hideWindow());
      break;
    case 4:
      // Minimize
      mcLaunchLog("Minimizing launcher as configured");
      runInUi(() => minimizeWindow());
      break;
    case 5:
      // Do nothing
      break;
  }

  // Increment launch count
  setConfig("SystemLaunchCount", Number(getConfig("SystemLaunchCount") || 0) + 1);
}

/**
 * Handle game process termination
 */
export function handleGameExit(code: number): void {
  mcLaunchLog(`Game process exited with code ${code}`);
  
  // Clean up resources - use local variables, not imports
  mcLaunchProcess = null;
  mcLaunchWatcher = null;
  
  // Show notification if configured
  if (getConfig("LaunchNotifyExit")) {
    showNotification("Minecraft has exited", `The game process has ended with code ${code}`);
  }
}

// Set the process and watcher references
export function setProcessReferences(process: any, watcher: any): void {
  mcLaunchProcess = process;
  mcLaunchWatcher = watcher;
}

// Placeholder implementation for functions - would be implemented elsewhere
function musicPause(): boolean {
  return true;
}

function musicResume(): boolean {
  return true;
}

function endProgram(restartApp: boolean): void {
  console.log(`Application ${restartApp ? "restarting" : "closing"}`);
}

function hideWindow(): void {
  console.log("Window hidden");
}

function minimizeWindow(): void {
  console.log("Window minimized");
}

function showNotification(title: string, message: string): void {
  console.log(`[Notification] ${title}: ${message}`);
}
