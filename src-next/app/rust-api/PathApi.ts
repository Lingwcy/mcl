import { invoke } from '@tauri-apps/api/core';

export interface ModPath {
  path: string;
  name: string;
  location: string;
}

export interface VersionPath {
  root: string;
  name: string;
  mods: ModPath[];
}

export interface RootPath {
  path: string;
  name?: string;
}

export class PathApi {
  static async getMinecraftPaths(): Promise<RootPath[]> {
    try {
      return await invoke<RootPath[]>('get_minecraft_paths');
    } catch (error) {
      console.error('Failed to get Minecraft paths:', error);
      return [];
    }
  }

  static async getScreenshots(rootPath: string): Promise<string[]> {
    try {
      return await invoke<string[]>('get_screenshots', { rootPath });
    } catch (error) {
      console.error('Failed to get screenshots:', error);
      return [];
    }
  }

  static async getVersions(rootPath: string): Promise<VersionPath[]> {
    try {
      return await invoke<VersionPath[]>('get_versions', { rootPath });
    } catch (error) {
      console.error('Failed to get versions:', error);
      return [];
    }
  }

  static async getAllMods(rootPath: string): Promise<ModPath[]> {
    try {
      return await invoke<ModPath[]>('get_all_mods', { rootPath });
    } catch (error) {
      console.error('Failed to get all mods:', error);
      return [];
    }
  }

  static async getRootMods(rootPath: string): Promise<ModPath[]> {
    try {
      return await invoke<ModPath[]>('get_root_mods', { rootPath });
    } catch (error) {
      console.error('Failed to get root mods:', error);
      return [];
    }
  }

  static async getVersionMods(rootPath: string, versionName: string): Promise<ModPath[]> {
    try {
      return await invoke<ModPath[]>('get_version_mods', { rootPath, versionName });
    } catch (error) {
      console.error('Failed to get version mods:', error);
      return [];
    }
  }
}
