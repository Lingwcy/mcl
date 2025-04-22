import * as path from 'path';
import * as fs from 'fs/promises';
import { createReadStream, createWriteStream, existsSync } from 'fs';
// Replace yauzl import with a type-only import or comment out if not needed yet
// import { ZipFile } from 'yauzl';
import { promisify } from 'util';
import { McLibToken } from './types';
import { mcLaunchLog } from './launch-controller';

// Define ZipFile type that would normally come from yauzl
interface ZipFile {
  on(event: string, listener: Function): void;
  readEntry(): void;
  openReadStream(entry: any, callback: Function): void;
}

/**
 * Extracts the native files for the game
 */
export async function mcLaunchNatives(loader: any): Promise<void> {
  // Create target directory
  const targetDir = getNativesFolder() + path.sep;
  await fs.mkdir(targetDir, { recursive: true });

  // Extract files
  mcLaunchLog("Extracting natives files");
  const existFiles: string[] = [];
  
  for (const native of loader.input as McLibToken[]) {
    if (!native.isNatives) continue;
    
    try {
      await extractNativeZip(native.localPath, targetDir, existFiles);
    } catch (ex: any) {
      mcLaunchLog(`Failed to open natives file: ${native.localPath}`);
      await fs.unlink(native.localPath);
      throw new Error(`Cannot open natives file (${native.localPath}), it may be corrupted. Please try launching the game again.`);
    }
  }

  // Delete extra files
  const allFiles = await fs.readdir(targetDir);
  for (const fileName of allFiles) {
    const fullPath = path.join(targetDir, fileName);
    if (!existFiles.includes(fullPath)) {
      try {
        mcLaunchLog(`Deleting: ${fullPath}`);
        await fs.unlink(fullPath);
      } catch (ex: any) {
        if (ex.code === 'EACCES') {
          mcLaunchLog("Access denied when deleting extra files, skipping deletion step");
          mcLaunchLog(`Actual error: ${ex.message}`);
          break;
        }
      }
    }
  }
}

/**
 * Extracts a native ZIP file
 */
async function extractNativeZip(zipPath: string, targetDir: string, existFiles: string[]): Promise<void> {
  return new Promise((resolve, reject) => {
    // We would need to use a ZIP library like yauzl or unzipper
    // This is a simplified implementation
    const yauzl = require('yauzl');
    yauzl.open(zipPath, { lazyEntries: true }, (err: any, zipfile: ZipFile) => {
      if (err) return reject(err);
      
      zipfile.on('entry', async (entry: any) => {
        const fileName = entry.fileName;
        if (!fileName.toLowerCase().endsWith('.dll')) {
          zipfile.readEntry();
          return;
        }

        // Actual file extraction
        const filePath = path.join(targetDir, fileName);
        existFiles.push(filePath);
        
        // Check if file exists and has the same size
        try {
          const stats = await fs.stat(filePath);
          if (stats.size === entry.uncompressedSize) {
            if (process.env.DEBUG) mcLaunchLog(`No need to extract: ${filePath}`);
            zipfile.readEntry();
            return;
          }
          
          // Delete existing file
          try {
            await fs.unlink(filePath);
          } catch (ex: any) {
            if (ex.code === 'EACCES') {
              mcLaunchLog(`Access denied when deleting original DLL, this usually means a Minecraft is running, skipping extraction: ${filePath}`);
              mcLaunchLog(`Actual error: ${ex.message}`);
              zipfile.readEntry();
              return;
            }
          }
        } catch (ex) {
          // File doesn't exist, continue with extraction
        }

        // Extract the file
        zipfile.openReadStream(entry, async (err: any, readStream: any) => {
          if (err) {
            zipfile.readEntry();
            return;
          }
          
          // Create write stream and pipe the data
          const writeStream = createWriteStream(filePath);
          readStream.pipe(writeStream);
          
          writeStream.on('close', () => {
            mcLaunchLog(`Extracted: ${filePath}`);
            zipfile.readEntry();
          });
        });
      });

      zipfile.on('end', () => {
        resolve();
      });

      zipfile.on('error', (err: any) => {
        reject(err);
      });

      zipfile.readEntry();
    });
  });
}

/**
 * Gets the natives folder path without a trailing separator
 */
export function getNativesFolder(): string {
  const result = path.join(mcVersionCurrent.path, `${mcVersionCurrent.name}-natives`);
  
  // Check if path contains only ASCII characters
  if (isValidEncoding(result)) return result;
  
  // Fallback to .minecraft path
  const fallback = path.join(process.env.APPDATA || '', '.minecraft', 'bin', 'natives');
  if (isValidEncoding(fallback)) return fallback;
  
  // Last resort
  return path.join(process.env.SystemDrive || 'C:', 'ProgramData', 'PCL', 'natives');
}

// Helper function to check if a string contains only ASCII characters
function isValidEncoding(str: string): boolean {
  return /^[\x00-\x7F]*$/.test(str);
}

// Import as needed from other modules
const mcVersionCurrent: any = null; // This would be imported from the version manager
