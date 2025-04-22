import { McLoginData, McLoginLegacy, McLoginMs, McLoginResult, McLoginServer, McLoginType } from '../types';
import { mcLaunchLog } from '../launch-controller';
import { getConfig, setConfig, getTimeTick, 
         markBuyHintDismissed, unlockTheme } from '../config';
import { hashString } from '../utils';

// Validation functions - these would be implemented based on the specific validation logic needed
const validateMsLogin = (): string => {
  return ""; // Empty string means validation passed
};

const validateLegacyLogin = (): string => {
  return ""; // Empty string means validation passed
};

const validateNideLogin = (): string => {
  return ""; // Empty string means validation passed
};

const validateAuthLogin = (): string => {
  return ""; // Empty string means validation passed
};

const validateMsLoginWithData = (data: McLoginData): string => {
  return ""; // Empty string means validation passed
};

const validateLegacyLoginWithData = (data: McLoginLegacy): string => {
  return ""; // Empty string means validation passed
};

const validateNideLoginWithData = (data: McLoginServer): string => {
  return ""; // Empty string means validation passed
};

const validateAuthLoginWithData = (data: McLoginServer): string => {
  return ""; // Empty string means validation passed
};

// Microsoft Login Step Functions
const msLoginStep1New = async (data: any): Promise<string[]> => {
  // Implementation for obtaining initial Microsoft OAuth tokens
  return ["accessToken", "refreshToken"];
};

const msLoginStep1Refresh = async (refreshToken: string): Promise<string[]> => {
  // Implementation for refreshing Microsoft OAuth tokens
  return ["accessToken", "refreshToken"];
};

const msLoginStep2 = async (accessToken: string): Promise<string> => {
  // Implementation for Xbox Live authentication
  return "xblToken";
};

const msLoginStep3 = async (xblToken: string): Promise<string[]> => {
  // Implementation for XSTS authentication
  return ["xstsToken", "uhs"];
};

const msLoginStep4 = async (tokens: string[]): Promise<string> => {
  // Implementation for getting Minecraft access token
  return "minecraftAccessToken";
};

const msLoginStep5 = async (accessToken: string): Promise<void> => {
  // Implementation for validating Minecraft ownership
};

const msLoginStep6 = async (accessToken: string): Promise<string[]> => {
  // Implementation for getting player profile
  return ["uuid", "username", "profileJson"];
};

// Local refresh time tracking
let localMsRefreshTime = 0;

// Function to update the refresh time in config if needed in the future
export function updateMsRefreshTime(time: number): void {
  localMsRefreshTime = time;
}

/**
 * Returns the player's MC username based on login information.
 * Returns null if username cannot be found.
 */
export function mcLoginName(): string | null {
  // Get username based on current login type
  switch (getConfig("LoginType")) {
    case McLoginType.Ms:
      if (getConfig("CacheMsV2Name") !== "") {
        return getConfig("CacheMsV2Name");
      }
      break;
    case McLoginType.Legacy:
      if (getConfig("LoginLegacyName") !== "") {
        return getConfig("LoginLegacyName").toString().split("¨")[0];
      }
      break;
    case McLoginType.Nide:
      if (getConfig("CacheNideName") !== "") {
        return getConfig("CacheNideName");
      }
      break;
    case McLoginType.Auth:
      if (getConfig("CacheAuthName") !== "") {
        return getConfig("CacheAuthName");
      }
      break;
  }
  
  // Look for any possible entries
  if (getConfig("CacheMsV2Name") !== "") return getConfig("CacheMsV2Name");
  if (getConfig("CacheNideName") !== "") return getConfig("CacheNideName");
  if (getConfig("CacheAuthName") !== "") return getConfig("CacheAuthName");
  if (getConfig("LoginLegacyName") !== "") return getConfig("LoginLegacyName").toString().split("¨")[0];
  
  return null;
}

/**
 * Checks if login is possible. If not, returns the error reason.
 */
export function mcLoginAble(): string {
  switch (getConfig("LoginType")) {
    case McLoginType.Ms:
      if (getConfig("CacheMsV2OAuthRefresh") === "") {
        return validateMsLogin();
      } else {
        return "";
      }
    case McLoginType.Legacy:
      return validateLegacyLogin();
    case McLoginType.Nide:
      if (getConfig("CacheNideAccess") === "") {
        return validateNideLogin();
      } else {
        return "";
      }
    case McLoginType.Auth:
      if (getConfig("CacheAuthAccess") === "") {
        return validateAuthLogin();
      } else {
        return "";
      }
    default:
      return "Unknown login type";
  }
}

/**
 * Checks if the provided login data can be used for login. If not, returns the error reason.
 * @param loginData Login data to check
 */
export function mcLoginAbleWithData(loginData: McLoginData): string {
  switch (loginData.type) {
    case McLoginType.Ms:
      return validateMsLoginWithData(loginData);
    case McLoginType.Legacy:
      return validateLegacyLoginWithData(loginData as McLoginLegacy);
    case McLoginType.Nide:
      return validateNideLoginWithData(loginData as McLoginServer);
    case McLoginType.Auth:
      return validateAuthLoginWithData(loginData as McLoginServer);
    default:
      return "Unknown login type";
  }
}

/**
 * Starts Microsoft login process
 */
export async function mcLoginMsStart(data: any): Promise<void> {
  const input = data.input as McLoginMs;
  const logUsername = input.userName;
  
  mcLaunchLog(`Login method: Official (${logUsername === "" ? "Not logged in" : logUsername})`);
  data.progress = 0.05;
  
  // Check if login is already complete
  if (!data.isForceRestarting && 
      input.accessToken !== "" && 
      (localMsRefreshTime > 0 && getTimeTick() - localMsRefreshTime < 1000 * 60 * 10)) {
    
    data.output = {
      accessToken: input.accessToken,
      name: input.userName,
      uuid: input.uuid,
      type: "Microsoft",
      clientToken: input.uuid,
      profileJson: input.profileJson
    };
    
    // Skip login
    markBuyHintDismissed();
    if (unlockTheme(10, false)) {
      showMessage("Thank you for supporting the official game!\nHidden theme 'Delay Red' has been unlocked!");
    }
    return;
  }
  
  // Try to login
  let oAuthTokens: string[];
  
  if (input.oAuthRefreshToken === "") {
    // No RefreshToken
    oAuthTokens = await msLoginStep1New(data);
  } else {
    // Have RefreshToken
    oAuthTokens = await msLoginStep1Refresh(input.oAuthRefreshToken);
    if (oAuthTokens[0] === "Relogin") {
      // Need to reopen login page authentication
      oAuthTokens = await msLoginStep1New(data);
    }
  }
  
  if (data.isAborted) {
    throw new Error("Thread interrupted");
  }
  
  data.progress = 0.25;
  
  if (data.isAborted) {
    throw new Error("Thread interrupted");
  }
  
  const oAuthAccessToken = oAuthTokens[0];
  const oAuthRefreshToken = oAuthTokens[1];
  
  const xblToken = await msLoginStep2(oAuthAccessToken);
  data.progress = 0.4;
  
  if (data.isAborted) {
    throw new Error("Thread interrupted");
  }
  
  const tokens = await msLoginStep3(xblToken);
  data.progress = 0.55;
  
  if (data.isAborted) {
    throw new Error("Thread interrupted");
  }
  
  const accessToken = await msLoginStep4(tokens);
  data.progress = 0.7;
  
  if (data.isAborted) {
    throw new Error("Thread interrupted");
  }
  
  await msLoginStep5(accessToken);
  data.progress = 0.85;
  
  if (data.isAborted) {
    throw new Error("Thread interrupted");
  }
  
  const result = await msLoginStep6(accessToken);
  data.progress = 0.98;
  
  // Output login result
  setConfig("CacheMsV2OAuthRefresh", oAuthRefreshToken);
  setConfig("CacheMsV2Access", accessToken);
  setConfig("CacheMsV2Uuid", result[0]);
  setConfig("CacheMsV2Name", result[1]);
  setConfig("CacheMsV2ProfileJson", result[2]);
  
  const msJson = JSON.parse(getConfig("LoginMsJson") || "{}");
  delete msJson[input.userName]; // In case player name changed
  msJson[result[1]] = oAuthRefreshToken;
  setConfig("LoginMsJson", JSON.stringify(msJson));
  
  data.output = {
    accessToken: accessToken,
    name: result[1],
    uuid: result[0],
    type: "Microsoft",
    clientToken: result[0],
    profileJson: result[2]
  };
  
  // Done - use local variable instead of imported one
  localMsRefreshTime = getTimeTick();
  mcLaunchLog("Microsoft login complete");
  
  markBuyHintDismissed();
  if (unlockTheme(10, false)) {
    showMessage("Thank you for supporting the official game!\nHidden theme 'Delay Red' has been unlocked!");
  }
}

/**
 * Generate legacy UUID from username
 */
function mcLoginLegacyUuid(name: string): string {
  const fullUuid = name.length.toString(16).padStart(16, "0") + 
                   hashString(name).toString(16).padStart(16, "0");
  
  return fullUuid.substring(0, 12) + "3" + 
         fullUuid.substring(13, 3) + "9" + 
         fullUuid.substring(17, 15);
}

/**
 * Fix level parameter for logError calls
 */
function logError(ex: any, message: string, level: string = "Normal"): void {
  console.error(`[${level}] ${message}`, ex);
}

/**
 * Fix showMessage parameter count
 */
function showMessage(message: string): void {
  console.log(message);
}
