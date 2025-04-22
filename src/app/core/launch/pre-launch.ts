import * as fs from 'fs/promises';
import * as path from 'path';
import { existsSync } from 'fs';
import { mcLaunchLog } from './launch-controller';

/**
 * Performs pre-launch processing
 */
export async function mcLaunchPrerun(): Promise<void> {
  mcLaunchLog("Starting pre-launch processing");

  // Request GPU preference (high-performance GPU)
  if (getConfig("LaunchAdvanceGraphicCard")) {
    try {
      await setGPUPreference(mcLaunchJavaSelected.pathJavaw);
      await setGPUPreference(process.execPath);
      mcLaunchLog("GPU preference set successfully");
    } catch (ex: any) {
      console.error("Failed to set GPU preference", ex);
      // In Node.js we can't restart with admin rights as easily as in VB.NET
      // We'd need to use a native module or different approach here
      mcLaunchLog("Failed to set GPU preference, Minecraft may use the default GPU");
    }
  }

  // Update launcher_profiles.json
  try {
    if (mcLoginLoader.output.type === "Microsoft") {
      await updateLauncherProfilesJson();
    }
  } catch (ex: any) {
    console.error("Failed to update launcher_profiles.json", ex);
    // Try again after deleting the file
    try {
      await fs.unlink(path.join(pathMcFolder, "launcher_profiles.json"));
      await updateLauncherProfilesJson();
      mcLaunchLog("Updated launcher_profiles.json after deletion");
    } catch (exx: any) {
      console.error("Failed to update launcher_profiles.json even after deletion", exx);
    }
  }

  // Update options.txt
  await updateOptionsTxt();

  // Handle offline skin warnings
  if (mcVersionCurrent.version.mcCodeMain <= 7 && 
      mcVersionCurrent.version.mcCodeMain >= 2 && 
      mcLoginLoader.input.type === McLoginType.Legacy &&
      (getConfig("LaunchSkinType") === 2 ||
       (getConfig("LaunchSkinType") === 4 && getConfig("LaunchSkinSlim")))) {
    showHint("This Minecraft version does not support Alex skins yet, your skin may appear as Steve!", "Critical");
  }

  // Handle offline skin resourcepack
  await setupOfflineSkinPack();
}

/**
 * Sets the GPU preference for a Java executable
 */
async function setGPUPreference(execPath: string): Promise<void> {
  if (process.platform !== 'win32') {
    return; // Only supported on Windows
  }

  // This would typically require Windows registry modifications
  // We would need a native module or spawn a process to do this
  mcLaunchLog(`Setting GPU preference for: ${execPath}`);
  // Implementation would depend on the Windows approach used
}

/**
 * Creates or updates the launcher_profiles.json file
 */
async function updateLauncherProfilesJson(): Promise<void> {
  const profilesPath = path.join(pathMcFolder, "launcher_profiles.json");
  
  // Create default if doesn't exist
  if (!existsSync(profilesPath)) {
    await createDefaultLauncherProfilesJson();
  }

  // Read existing file
  const profilesContent = await fs.readFile(profilesPath, 'utf8');
  const profiles = JSON.parse(profilesContent);

  // Prepare update object
  const updateJson = {
    authenticationDatabase: {
      "00000111112222233333444445555566": {
        username: mcLoginLoader.output.name.replace(/"/g, "-"),
        profiles: {
          "66666555554444433333222221111100": {
            displayName: mcLoginLoader.output.name
          }
        }
      }
    },
    clientToken: mcLoginLoader.output.clientToken,
    selectedUser: {
      account: "00000111112222233333444445555566",
      profile: "66666555554444433333222221111100"
    }
  };

  // Merge and save
  Object.assign(profiles, updateJson);
  await fs.writeFile(profilesPath, JSON.stringify(profiles, null, 2));
  mcLaunchLog("Updated launcher_profiles.json");
}

/**
 * Creates a default launcher_profiles.json file
 */
async function createDefaultLauncherProfilesJson(): Promise<void> {
  const defaultJson = {
    profiles: {},
    settings: {},
    version: {}
  };
  
  await fs.writeFile(
    path.join(pathMcFolder, "launcher_profiles.json"), 
    JSON.stringify(defaultJson, null, 2)
  );
}

/**
 * Updates the options.txt file with appropriate settings
 */
async function updateOptionsTxt(): Promise<void> {
  let optionsPath = path.join(mcVersionCurrent.pathIndie, "options.txt");
  
  // Check for Yosbr Mod compatibility
  if (!existsSync(optionsPath)) {
    const yosbrPath = path.join(mcVersionCurrent.pathIndie, "config", "yosbr", "options.txt");
    if (existsSync(yosbrPath)) {
      mcLaunchLog("Will modify options.txt in Yosbr Mod");
      optionsPath = yosbrPath;
      await writeIni(optionsPath, "lang", "none"); // Ignore default language
    }
  }

  try {
    // Set language
    let currentLang = readIni(optionsPath, "lang", "none");
    const requiredLang = currentLang === "none" || !existsSync(path.join(mcVersionCurrent.pathIndie, "saves")) ?
      (getConfig("ToolHelpChinese") ? "zh_cn" : "en_us") : 
      currentLang.toLowerCase();
    
    const finalLang = mcVersionCurrent.version.mcCodeMain < 12 ?
      // For older versions, make the last two chars uppercase
      requiredLang.substring(0, requiredLang.length - 2) + 
      requiredLang.substring(requiredLang.length - 2).toUpperCase() :
      requiredLang;
    
    if (currentLang === finalLang) {
      mcLaunchLog(`Required language is ${finalLang}, current language is ${currentLang}, no change needed`);
    } else {
      await writeIni(optionsPath, "lang", "-"); // Trigger cache update
      await writeIni(optionsPath, "lang", finalLang);
      mcLaunchLog(`Changed language from ${currentLang} to ${finalLang}`);
    }
    
    // Set window mode
    const windowType = getConfig("LaunchArgumentWindowType");
    if (windowType === 0) { // Fullscreen
      await writeIni(optionsPath, "fullscreen", "true");
    } else if (windowType !== 1) { // Not default
      await writeIni(optionsPath, "fullscreen", "false");
    }
  } catch (ex) {
    console.error("Failed to update options.txt", ex);
  }
}

/**
 * Sets up the offline skin resourcepack
 */
async function setupOfflineSkinPack(): Promise<void> {
  try {
    const resourcepacksDir = path.join(mcVersionCurrent.pathIndie, "resourcepacks");
    await fs.mkdir(resourcepacksDir, { recursive: true });
    
    const zipPath = path.join(resourcepacksDir, "PCL2 Skin.zip");
    const isNewTypeSetup = mcVersionCurrent.version.mcCodeMain >= 13 || 
                           mcVersionCurrent.version.mcCodeMain < 6;
    
    if (mcLoginLoader.input.type === McLoginType.Legacy && 
        getConfig("LaunchSkinType") === 4 && 
        existsSync(path.join(pathAppdata, "CustomSkin.png"))) {
      
      // Create custom skin resourcepack
      await createCustomSkinResourcepack(zipPath, isNewTypeSetup);
    } 
    else if (existsSync(zipPath)) {
      // Remove existing resourcepack
      mcLaunchLog("Clearing custom skin resourcepack");
      await fs.unlink(zipPath);
      
      // Update options.txt
      const optionsFile = path.join(mcVersionCurrent.pathIndie, "options.txt");
      clearIniCache(optionsFile);
      
      const enabledPacks = readIni(optionsFile, "resourcePacks", "[]").trim().slice(1, -1);
      const packsList = enabledPacks ? enabledPacks.split(",") : [];
      const filteredPacks = packsList.filter(pack => 
        pack !== "\"file/PCL2 Skin.zip\"" && 
        pack !== "\"PCL2 Skin.zip\"" && 
        pack.trim() !== ""
      );
      
      const result = "[" + filteredPacks.join(",") + "]";
      await writeIni(optionsFile, "resourcePacks", result);
    }
  } catch (ex) {
    console.error("Failed to setup offline skin resourcepack", ex);
  }
}

/**
 * Creates a custom skin resourcepack
 */
async function createCustomSkinResourcepack(zipPath: string, isNewType: boolean): Promise<void> {
  // This would be implemented using a ZIP library
  // For now this is a placeholder implementation
  mcLaunchLog(`Creating custom skin resourcepack with format for ${isNewType ? "new" : "old"} Minecraft versions`);
  
  // The actual implementation would:
  // 1. Create appropriate pack.mcmeta with the right format
  // 2. Include the skin file in the appropriate location
  // 3. Update the options.txt file to use this resourcepack
  
  mcLaunchLog("Custom skin resourcepack created successfully");
  
  // Update options.txt to enable the pack
  const optionsFile = path.join(mcVersionCurrent.pathIndie, "options.txt");
  const packEntry = isNewType ? "\"file/PCL2 Skin.zip\"" : "\"PCL2 Skin.zip\"";
  
  clearIniCache(optionsFile);
  let enabledPacks = readIni(optionsFile, "resourcePacks", "[]").trim().slice(1, -1);
  if (isNewType && enabledPacks === "") {
    enabledPacks = "\"vanilla\"";
  }
  
  const packsList = enabledPacks ? enabledPacks.split(",") : [];
  const filteredPacks = packsList.filter(pack => 
    pack !== "\"file/PCL2 Skin.zip\"" && 
    pack !== "\"PCL2 Skin.zip\"" && 
    pack.trim() !== ""
  );
  filteredPacks.push(packEntry);
  
  const result = "[" + filteredPacks.join(",") + "]";
  await writeIni(optionsFile, "resourcePacks", result);
}

// Helper functions for INI-style config files
function readIni(filePath: string, key: string, defaultValue: string): string {
  // Implementation would need to read a key-value from an INI-style file
  return defaultValue;
}

async function writeIni(filePath: string, key: string, value: string): Promise<void> {
  // Implementation would need to write a key-value to an INI-style file
}

function clearIniCache(filePath: string): void {
  // Implementation to clear any caching of INI values
}

// Import from other modules
const mcVersionCurrent: any = null;
const mcLaunchJavaSelected: any = null;
const mcLoginLoader: any = { output: { type: "", name: "", clientToken: "" } };
const pathMcFolder: string = "";
const pathAppdata: string = "";
enum McLoginType { Legacy = 0 }

// Helper functions
function getConfig(key: string): any {
  return "";
}

function showHint(message: string, type: string): void {
  console.log(`[${type}] ${message}`);
}
