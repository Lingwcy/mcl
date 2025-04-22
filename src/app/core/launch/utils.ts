import * as path from 'path';
import * as fs from 'fs/promises';

/**
 * Shortens a path by converting it to short form if necessary
 */
export function shortenPath(pathToShorten: string): string {
  if (!pathToShorten) return pathToShorten;
  
  // In Windows, convert long paths to short paths if they contain spaces or special characters
  if (process.platform === 'win32' && 
      (pathToShorten.includes(' ') || /[^\x00-\x7F]/.test(pathToShorten))) {
    try {
      // This would use a native module in actual implementation
      // For Windows, we could use the 'win32-api' module to call GetShortPathNameW
      // For simplicity here, just return the path with quotes if needed
      return pathToShorten;
    } catch {
      return pathToShorten;
    }
  }
  
  return pathToShorten;
}

/**
 * Checks if a file exists
 */
export async function fileExists(filePath: string): Promise<boolean> {
  try {
    await fs.access(filePath);
    return true;
  } catch {
    return false;
  }
}

/**
 * Secret filter to hide sensitive information in logs
 */
export function secretFilter(text: string, replacement: string): string {
  if (!text) return text;
  
  // Replace access tokens, client tokens, and other sensitive info with replacement characters
  return text
    .replace(/accessToken[":]+"([^"]+)"/g, `accessToken":"${replacement.repeat(10)}"`)
    .replace(/clientToken[":]+"([^"]+)"/g, `clientToken":"${replacement.repeat(10)}"`)
    .replace(/-Dauthlibinjector\.yggdrasil\.prefetched=([^\s]+)/g, `-Dauthlibinjector.yggdrasil.prefetched=${replacement.repeat(5)}`);
}

/**
 * A simple sleep function that returns a promise
 */
export function sleep(ms: number): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, ms));
}

/**
 * Run a function in the UI thread
 */
export function runInUi(callback: () => void): void {
  setTimeout(callback, 0);
}

/**
 * Run a function in the UI thread and wait for it to complete
 */
export function runInUiWait<T>(callback: () => T): Promise<T> {
  return new Promise((resolve) => {
    setTimeout(() => {
      resolve(callback());
    }, 0);
  });
}

/**
 * Calculate a hash for a string
 */
export function hashString(str: string): number {
  let hash = 0;
  if (str.length === 0) return hash;
  
  for (let i = 0; i < str.length; i++) {
    const char = str.charCodeAt(i);
    hash = ((hash << 5) - hash) + char;
    hash = hash & hash; // Convert to 32bit integer
  }
  
  return Math.abs(hash);
}

/**
 * Check if a string uses only ASCII encoding
 */
export function isValidEncoding(str: string): boolean {
  return /^[\x00-\x7F]*$/.test(str);
}

/**
 * Show a hint message to the user
 */
export function showHint(message: string, type: string): void {
  console.log(`[${type}] ${message}`);
}
