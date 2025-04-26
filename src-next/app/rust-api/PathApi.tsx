import { invoke } from '@tauri-apps/api/core';

export interface RootPath {
  path: string;
  display_name: string;
}

export interface VersionPath {
  name: string;
  path: string;
  mod_count: number;
}

export interface ModPath {
  name: string;
  path: string;
  location: string;
}

export class PathApi {
  static async getMinecraftPaths(): Promise<RootPath[]> {
    return invoke<RootPath[]>("get_all_minecraft_paths");
  }

  static async getVersions(rootPath: string): Promise<VersionPath[]> {
    return invoke<VersionPath[]>("get_minecraft_versions_for_path", { path: rootPath });
  }

  static async getRootMods(rootPath: string): Promise<ModPath[]> {
    return invoke<ModPath[]>("get_minecraft_mods_for_version", { 
      rootPath: rootPath, 
      versionName: "global" 
    });
  }

  static async getVersionMods(rootPath: string, versionName: string): Promise<ModPath[]> {
    return invoke<ModPath[]>("get_minecraft_mods_for_version", { 
      rootPath: rootPath, 
      versionName: versionName 
    });
  }

  static async addMinecraftPath(rootPath: string): Promise<string> {
    return invoke("initialize_game_path", { rootPath: rootPath });
  }

  static async addMinecraftPathWithName(rootPath: string, name: string): Promise<string> {
    return invoke<string>("initialize_game_path_with_name", { 
      rootPath: rootPath, 
      name: name 
    });
  }
}
