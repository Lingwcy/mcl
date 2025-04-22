/**
 * Launch options for Minecraft
 */
export class McLaunchOptions {
  /**
   * Force specify server IP to join after launch.
   * Default: undefined. Uses version settings.
   */
  serverIp: string | null = null;
  
  /**
   * Save launch script to this path, then cancel launch. This will also change prompts during launch.
   * Default: undefined. Don't save.
   */
  saveBatch: string | null = null;
  
  /**
   * Force specify MC version to launch.
   * Default: undefined. Uses McVersionCurrent.
   */
  version: any = null; // Replace 'any' with proper McVersion type when implemented
  
  /**
   * Extra launch arguments.
   */
  extraArgs: string[] = [];
}

/**
 * Minecraft library token for handling libraries and natives
 */
export interface McLibToken {
  /**
   * Name of the library in Maven format (e.g. "org.lwjgl:lwjgl:3.2.2")
   */
  name: string | null;
  
  /**
   * Local path to the library JAR
   */
  localPath: string;
  
  /**
   * Whether this is a native library
   */
  isNatives: boolean;
  
  /**
   * URL to download from
   */
  downloadUrl?: string;
}

/**
 * Login types enum
 */
export enum McLoginType {
  Legacy = 0,
  Nide = 2,
  Auth = 3,
  Ms = 5
}

/**
 * Base class for login data
 */
export abstract class McLoginData {
  /**
   * Login type
   */
  type: McLoginType;

  constructor(type: McLoginType) {
    this.type = type;
  }

  equals(obj: any): boolean {
    return obj !== null && obj.hashCode() === this.hashCode();
  }

  abstract hashCode(): number;
}

/**
 * Server login data
 */
export class McLoginServer extends McLoginData {
  /**
   * Login username
   */
  userName: string;

  /**
   * Login password
   */
  password: string;

  /**
   * Login server base URL
   */
  baseUrl: string;

  /**
   * Token used for login, can only be "Auth" or "Nide", used for cache storage
   */
  token: string;

  /**
   * Login method description, e.g. "正版", "统一通行证"
   */
  description: string;

  /**
   * Whether to force user to reselect profile in this login, currently only works for Authlib-Injector
   */
  forceReselectProfile: boolean = false;

  constructor(type: McLoginType) {
    super(type);
    this.userName = '';
    this.password = '';
    this.baseUrl = '';
    this.token = '';
    this.description = '';
  }

  hashCode(): number {
    // Implementation of hash function would go here
    return hashString(this.userName + this.password + this.baseUrl + this.token + this.type.toString());
  }
}

/**
 * Microsoft login data
 */
export class McLoginMs extends McLoginData {
  /**
   * Cached OAuth Refresh Token. Empty string if none.
   */
  oAuthRefreshToken: string = "";
  
  accessToken: string = "";
  uuid: string = "";
  userName: string = "";
  profileJson: string = "";

  constructor() {
    super(McLoginType.Ms);
  }

  hashCode(): number {
    return hashString(
      this.oAuthRefreshToken + this.accessToken + this.uuid + this.userName + this.profileJson
    );
  }
}

/**
 * Legacy login data
 */
export class McLoginLegacy extends McLoginData {
  /**
   * Login username
   */
  userName: string = "";
  
  /**
   * Skin type
   */
  skinType: number = 0;
  
  /**
   * If using official skin, this is the skin name
   */
  skinName: string = "";

  constructor() {
    super(McLoginType.Legacy);
  }

  hashCode(): number {
    return hashString(this.userName + this.skinType + this.skinName + this.type.toString());
  }
}

/**
 * Login result structure
 */
export interface McLoginResult {
  name: string;
  uuid: string;
  accessToken: string;
  type: string;
  clientToken: string;
  /**
   * Profile information returned during Microsoft login
   */
  profileJson: string;
}

/**
 * Simple string hash function
 */
function hashString(str: string): number {
  let hash = 0;
  if (str.length === 0) return hash;
  
  for (let i = 0; i < str.length; i++) {
    const char = str.charCodeAt(i);
    hash = ((hash << 5) - hash) + char;
    hash = hash & hash; // Convert to 32bit integer
  }
  
  return Math.abs(hash);
}
