"use client"
import { createContext, useState, useEffect, ReactNode, useCallback } from "react";
import { PathApi, ModPath, VersionPath, RootPath } from "../rust-api/PathApi";

interface PathContextType {
  roots: RootPath[] | undefined;
  selectedRoot: RootPath | undefined;
  versions: VersionPath[] | undefined;
  rootMods: ModPath[] | undefined;
  loading: boolean;
  error: string | null;
  selectRoot: (rootPath: RootPath) => Promise<void>;
  refreshPaths: () => Promise<void>;
  getVersionMods: (versionName: string) => Promise<ModPath[]>;
}

export const PathContext = createContext<PathContextType>({
  roots: undefined,
  selectedRoot: undefined,
  versions: undefined,
  rootMods: undefined,
  loading: false,
  error: null,
  selectRoot: async () => {},
  refreshPaths: async () => {},
  getVersionMods: async () => [],
});

interface PathProviderProps {
  children: ReactNode;
}

export const PathProvider = ({ children }: PathProviderProps) => {
  const [roots, setRoots] = useState<RootPath[] | undefined>(undefined);
  const [selectedRoot, setSelectedRoot] = useState<RootPath | undefined>(undefined);
  const [versions, setVersions] = useState<VersionPath[] | undefined>(undefined);
  const [rootMods, setRootMods] = useState<ModPath[] | undefined>(undefined);
  const [loading, setLoading] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);

  // Use useCallback to avoid recreation of this function on each render
  const refreshPaths = useCallback(async () => {
    console.log("Refreshing paths...");
    setLoading(true);
    setError(null);
    try {
      const paths = await PathApi.getMinecraftPaths();
      console.log("Fetched paths:", paths);
      setRoots(paths);
      
      // If there's a selected root, check if it still exists in the updated paths
      if (selectedRoot) {
        const stillExists = paths.some(p => p.path === selectedRoot.path);
        if (!stillExists && paths.length > 0) {
          // If selected root no longer exists but we have other paths, select the first one
          await selectRoot(paths[0]);
        } else if (stillExists) {
          // If it still exists, refresh its data
          const updatedRoot = paths.find(p => p.path === selectedRoot.path);
          if (updatedRoot) {
            await selectRoot(updatedRoot);
          }
        }
      } else if (paths.length > 0) {
        // If no root was selected before but we have paths now, select the first one
        await selectRoot(paths[0]);
      }
    } catch (err) {
      console.error("Failed to refresh paths:", err);
      setError(`Failed to load Minecraft paths: ${err}`);
    } finally {
      setLoading(false);
    }
  }, [selectedRoot]);

  const selectRoot = async (rootPath: RootPath) => {
    console.log("Selecting root:", rootPath);
    setSelectedRoot(rootPath);
    setLoading(true);
    try {
      const [versionsData, modsData] = await Promise.all([
        PathApi.getVersions(rootPath.path),
        PathApi.getRootMods(rootPath.path)
      ]);
      console.log("Fetched versions:", versionsData);
      console.log("Fetched mods:", modsData);
      setVersions(versionsData);
      setRootMods(modsData);
    } catch (err) {
      console.error("Failed to select root:", err);
      setError(`Failed to load data for ${rootPath.path}: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  const getVersionMods = async (versionName: string): Promise<ModPath[]> => {
    if (!selectedRoot) return [];
    try {
      return await PathApi.getVersionMods(selectedRoot.path, versionName);
    } catch (err) {
      setError(`Failed to load mods for version ${versionName}: ${err}`);
      return [];
    }
  };

  // Initial load on component mount
  useEffect(() => {
    refreshPaths();
  }, []);

  return (
    <PathContext.Provider
      value={{
        roots,
        selectedRoot,
        versions,
        rootMods,
        loading,
        error,
        selectRoot,
        refreshPaths,
        getVersionMods,
      }}
    >
      {children}
    </PathContext.Provider>
  );
};
