import { McLibToken } from './types';
import path from 'path';
import fs from 'fs/promises';
import { mcLaunchLog } from './launch-controller';
import { extractJavaWrapper } from './java';
import { getNativesFolder } from './natives';
import { secretFilter, shortenPath, showHint } from './utils';
import { 
  mcVersionCurrent, pathMcFolder, mcLaunchJavaSelected, currentLaunchOptions,
  pathPure, pathAppdata, mcLoginLoader, getConfig, hiperEnabled
} from './config';
import { McLoginType } from './types';

// Store the final launch argument string
let mcLaunchArgument: string = "";

/**
 * Main method for getting launch arguments by combining Jvm, Game, and Replace parts
 */
export async function getMcLaunchArguments(loader: any): Promise<McLibToken[]> {
  mcLaunchLog("Starting to get Minecraft launch arguments");
  
  // Get basic strings and parameter information
  let args = "";
  
  if (mcVersionCurrent.jsonObject.arguments && mcVersionCurrent.jsonObject.arguments.jvm) {
    mcLaunchLog("Getting new JVM arguments");
    args = await mcLaunchArgumentsJvmNew(mcVersionCurrent);
    mcLaunchLog("New JVM arguments obtained:");
    mcLaunchLog(args);
  } else {
    mcLaunchLog("Getting old JVM arguments");
    args = await mcLaunchArgumentsJvmOld(mcVersionCurrent);
    mcLaunchLog("Old JVM arguments obtained:");
    mcLaunchLog(args);
  }
  
  if (mcVersionCurrent.jsonObject.minecraftArguments && mcVersionCurrent.jsonObject.minecraftArguments !== "") {
    mcLaunchLog("Getting old Game arguments");
    args += " " + await mcLaunchArgumentsGameOld(mcVersionCurrent);
    mcLaunchLog("Old Game arguments obtained");
  }
  
  if (mcVersionCurrent.jsonObject.arguments && mcVersionCurrent.jsonObject.arguments.game) {
    mcLaunchLog("Getting new Game arguments");
    args += " " + await mcLaunchArgumentsGameNew(mcVersionCurrent);
    mcLaunchLog("New Game arguments obtained");
  }
  
  // Encoding arguments
  if (mcLaunchJavaSelected.versionCode > 8) {
    if (!args.includes("-Dfile.encoding=")) args += " -Dfile.encoding=UTF-8";
    if (!args.includes("-Dstdout.encoding=")) args += " -Dstdout.encoding=UTF-8";
    if (!args.includes("-Dstderr.encoding=")) args += " -Dstderr.encoding=UTF-8";
  }
  
  // Replace arguments
  const replaceArguments = await mcLaunchArgumentsReplace(mcVersionCurrent, loader);
  
  if (!replaceArguments["${version_type}"]) {
    // If custom info is empty, remove this part
    args = args.replace(" --versionType ${version_type}", "");
    replaceArguments["${version_type}"] = "\"\"";
  }
  
  for (const [key, value] of Object.entries(replaceArguments)) {
    const valueStr = String(value);
    args = args.replace(key, valueStr.includes(" ") || valueStr.includes(":\\") ? `"${valueStr}"` : valueStr);
  }
  
  // MJSB
  args = args.replace(" -Dos.name=Windows 10", " -Dos.name=\"Windows 10\"");
  
  // Fullscreen
  if (getConfig("LaunchArgumentWindowType") === 0) args += " --fullscreen";
  
  // Extra arguments passed via Options
  for (const arg of currentLaunchOptions.extraArgs) {
    args += " " + arg.trim();
  }
  
  // Server connection
  const server = currentLaunchOptions.serverIp || getConfig("VersionServerEnter", { version: mcVersionCurrent });
  if (server && server.length > 0) {
    if (mcVersionCurrent.releaseTime > new Date(2023, 3, 4)) {
      // QuickPlay
      args += ` --quickPlayMultiplayer "${server}"`;
    } else {
      // Old versions
      if (server.includes(":")) {
        // Contains port
        args += ` --server ${server.split(":")[0]} --port ${server.split(":")[1]}`;
      } else {
        // No port
        args += ` --server ${server} --port 25565`;
      }
      
      if (mcVersionCurrent.version.hasOptiFine) {
        showHint("OptiFine may not be compatible with automatic server connection, which could cause texture loss or even game crashes!", "Critical");
      }
    }
  }
  
  // Custom arguments
  const argumentGame = getConfig("VersionAdvanceGame", { version: mcVersionCurrent });
  args += " " + (argumentGame || getConfig("LaunchAdvanceGame"));
  
  // Output
  mcLaunchLog("Minecraft launch arguments:");
  mcLaunchLog(args);
  mcLaunchArgument = args;
  
  // Return library tokens from replace arguments
  return loader.output;
}

// Save the function by its old name for backwards compatibility
export const mcLaunchArgumentMain = getMcLaunchArguments;

/**
 * Implementation of various argument construction functions
 */
async function mcLaunchArgumentsJvmOld(version: any): Promise<string> {
  // Store launch arguments with space separator
  const dataList: string[] = [];
  
  // Add fixed arguments
  dataList.push("-XX:HeapDumpPath=MojangTricksIntelDriversForPerformance_javaw.exe_minecraft.exe.heapdump");
  
  let argumentJvm = getConfig("VersionAdvanceJvm", { version: mcVersionCurrent });
  if (argumentJvm === "") argumentJvm = getConfig("LaunchAdvanceJvm");
  
  if (!argumentJvm.includes("-Dlog4j2.formatMsgNoLookups=true")) {
    argumentJvm += " -Dlog4j2.formatMsgNoLookups=true";
  }
  
  // Clean up from #3511
  argumentJvm = argumentJvm.replace(" -XX:MaxDirectMemorySize=256M", "");
  
  dataList.unshift(argumentJvm); // Variable JVM parameters
  
  dataList.push(`-Xmn${Math.floor(getVersionRam(mcVersionCurrent, !mcLaunchJavaSelected.is64Bit) * 1024 * 0.15)}m`);
  dataList.push(`-Xmx${Math.floor(getVersionRam(mcVersionCurrent, !mcLaunchJavaSelected.is64Bit) * 1024)}m`);
  dataList.push(`"-Djava.library.path=${getNativesFolder()}"`);
  dataList.push("-cp ${classpath}"); // Add libraries to launch parameters
  
  // Unified Pass
  if (mcLoginLoader.output.type === "Nide") {
    dataList.unshift(`-Dnide8auth.client=true -javaagent:"${pathAppdata}nide8auth.jar"=${getConfig("VersionServerNide", { version: mcVersionCurrent })}`);
  }
  
  // Authlib-Injector
  if (mcLoginLoader.output.type === "Auth") {
    const server = mcLoginLoader.input.type === McLoginType.Legacy
      ? "http://hiperauth.tech/api/yggdrasil-hiper/" // HiPer login
      : getConfig("VersionServerAuthServer", mcVersionCurrent);
    
    try {
      const response = await netGetCodeByRequestRetry(server, "utf-8");
      dataList.unshift(`-javaagent:"${pathPure}authlib-injector.jar"=${server}`
                     + " -Dauthlibinjector.side=client"
                     + ` -Dauthlibinjector.yggdrasil.prefetched=${Buffer.from(response, 'utf8').toString('base64')}`);
    } catch (ex) {
      throw new Error(`Cannot connect to third-party login server (${server || null})`);
    }
  }
  
  // Add Java Wrapper as main Jar
  if (!getConfig("LaunchAdvanceDisableJLW") && !getConfig("VersionAdvanceDisableJLW", mcVersionCurrent)) {
    if (mcLaunchJavaSelected.versionCode >= 9) {
      dataList.push("--add-exports cpw.mods.bootstraplauncher/cpw.mods.bootstraplauncher=ALL-UNNAMED");
    }
    
    dataList.push(`-Doolloo.jlw.tmpdir="${pathPure.replace(/\\$/, "")}"`);
    dataList.push(`-jar "${await extractJavaWrapper()}"`);
  }
  
  // Add MainClass
  if (!version.jsonObject.mainClass) {
    throw new Error("No mainClass item in version json!");
  } else {
    dataList.push(version.jsonObject.mainClass);
  }
  
  return dataList.join(" ");
}

async function mcLaunchArgumentsJvmNew(version: any): Promise<string> {
  // Implementation would be added here
  // For now, returning a placeholder
  return "-Xmx2G -XX:+UnlockExperimentalVMOptions -XX:+UseG1GC -XX:G1NewSizePercent=20 -XX:G1ReservePercent=20 -XX:MaxGCPauseMillis=50 -XX:G1HeapRegionSize=32M";
}

async function mcLaunchArgumentsGameOld(version: any): Promise<string> {
  // Implementation would be added here
  // For now, returning a placeholder
  return "--username ${auth_player_name} --version ${version_name} --gameDir ${game_directory} --assetsDir ${assets_root} --assetIndex ${assets_index_name} --uuid ${auth_uuid} --accessToken ${auth_access_token} --userType ${user_type} --versionType ${version_type}";
}

async function mcLaunchArgumentsGameNew(version: any): Promise<string> {
  // Implementation would be added here
  // For now, returning a placeholder
  return "--username ${auth_player_name} --version ${version_name} --gameDir ${game_directory} --assetsDir ${assets_root} --assetIndex ${assets_index_name} --uuid ${auth_uuid} --accessToken ${auth_access_token} --userType ${user_type} --versionType ${version_type}";
}

async function mcLaunchArgumentsReplace(version: any, loader: any): Promise<Record<string, string>> {
  // Implementation would be added here
  // For now, returning basic placeholders
  return {
    "${classpath_separator}": ";",
    "${natives_directory}": shortenPath(getNativesFolder()),
    "${library_directory}": shortenPath(path.join(pathMcFolder, "libraries")),
    "${libraries_directory}": shortenPath(path.join(pathMcFolder, "libraries")),
    "${launcher_name}": "PCL",
    "${launcher_version}": "2.0.0",
    "${version_name}": version.name,
    "${version_type}": getConfig("VersionArgumentInfo", { version: mcVersionCurrent }) || getConfig("LaunchArgumentInfo"),
    "${game_directory}": shortenPath(version.pathIndie.slice(0, -1)),
    "${assets_root}": shortenPath(path.join(pathMcFolder, "assets")),
    "${user_properties}": "{}",
    "${auth_player_name}": mcLoginLoader.output.name,
    "${auth_uuid}": mcLoginLoader.output.uuid,
    "${auth_access_token}": mcLoginLoader.output.accessToken,
    "${access_token}": mcLoginLoader.output.accessToken,
    "${auth_session}": mcLoginLoader.output.accessToken,
    "${user_type}": "msa",
    "${resolution_width}": "854",
    "${resolution_height}": "480",
    "${game_assets}": shortenPath(path.join(pathMcFolder, "assets/virtual/legacy")),
    "${assets_index_name}": "1.19",
    "${classpath}": "placeholder_classpath"
  };
}

/**
 * Creates a file with launch script and returns the path
 */
export async function saveLaunchScript(savePath: string | null): Promise<string> {
  const scriptPath = savePath || path.join(process.env.TEMP || '', "PCL", "LatestLaunch.bat");
  
  // Ensure directory exists
  await fs.mkdir(path.dirname(scriptPath), { recursive: true });
  
  // Get custom commands
  const customCommandGlobal = argumentReplace(getConfig("LaunchAdvanceRun") || "", true);
  const customCommandVersion = argumentReplace(getConfig("VersionAdvanceRun", { version: mcVersionCurrent }) || "", true);
  
  // Create script content
  const cmdString = 
    `${mcLaunchJavaSelected.versionCode > 8 ? "chcp 65001>nul\r\n" : ""}` +
    "@echo off\r\n" +
    `title Launch - ${mcVersionCurrent.name}\r\n` +
    "echo Game is launching, please wait...\r\n" +
    `set APPDATA="${shortenPath(mcVersionCurrent.pathIndie)}"\r\n` +
    `cd /D "${shortenPath(mcVersionCurrent.pathIndie)}"\r\n` +
    `${customCommandGlobal}\r\n` +
    `${customCommandVersion}\r\n` +
    `"${mcLaunchJavaSelected.pathJava}" ${mcLaunchArgument}\r\n` +
    "echo Game has exited.\r\n" +
    "pause";
    
  // Write to file
  await fs.writeFile(
    scriptPath, 
    secretFilter(cmdString, "F"), 
    { encoding: 'utf8' }
  );
  
  return scriptPath;
}

/**
 * Process custom commands for pre-launch
 */
export async function executeCustomCommands(loader: any): Promise<void> {
  // Implementation remains the same as in your original file
  // ...existing code...
}

/**
 * Processes replacement tokens in strings for launch arguments
 */
export function argumentReplace(raw: string, replaceTimeAndDate: boolean): string {
  if (!raw) return raw;
  
  // Path replacements
  raw = raw.replace(/\{minecraft\}/g, pathMcFolder);
  raw = raw.replace(/\{verpath\}/g, mcVersionCurrent.path);
  raw = raw.replace(/\{verindie\}/g, mcVersionCurrent.pathIndie);
  raw = raw.replace(/\{java\}/g, mcLaunchJavaSelected.pathFolder);
  
  // Standard replacements
  raw = raw.replace(/\{user\}/g, mcLoginLoader.output.name);
  raw = raw.replace(/\{uuid\}/g, mcLoginLoader.output.uuid);
  
  if (replaceTimeAndDate) {
    const now = new Date();
    raw = raw.replace(/\{date\}/g, now.toLocaleDateString());
    raw = raw.replace(/\{time\}/g, now.toLocaleTimeString());
  }
  
  // Login type replacement
  let loginType = "";
  switch (mcLoginLoader.input.type) {
    case McLoginType.Legacy:
      loginType = hiperEnabled ? "Online Offline" : "Offline";
      break;
    case McLoginType.Ms:
      loginType = "Microsoft";
      break;
    case McLoginType.Nide:
      loginType = "Unified Pass";
      break;
    case McLoginType.Auth:
      loginType = "Authlib-Injector";
      break;
  }
  raw = raw.replace(/\{login\}/g, loginType);
  
  // Version replacements
  raw = raw.replace(/\{name\}/g, mcVersionCurrent.name);
  
  if (["unknown", "old", "pending"].includes(mcVersionCurrent.version.mcName?.toLowerCase() || "")) {
    raw = raw.replace(/\{version\}/g, mcVersionCurrent.name);
  } else {
    raw = raw.replace(/\{version\}/g, mcVersionCurrent.version.mcName);
  }
  
  raw = raw.replace(/\{path\}/g, process.cwd());
  
  return raw;
}

/**
 * Get RAM allocation based on version
 */
function getVersionRam(version: any, is32Bit: boolean): number {
  // Implementation would be added here
  // For now, returning a default value
  return 2; // 2GB default
}

/**
 * Perform network request with retry capability
 */
async function netGetCodeByRequestRetry(url: string, encoding: string): Promise<string> {
  // Implementation would be added here
  // For now, returning a placeholder
  return "placeholder-response";
}

export { mcLaunchArgument };
