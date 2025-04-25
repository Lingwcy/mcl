"use client"
import { createContext, useState, useEffect, ReactNode } from "react";
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

  const refreshPaths = async () => {
    setLoading(true);
    setError(null);
    try {
      const paths = await PathApi.getMinecraftPaths();
      setRoots(paths);
      if (paths.length > 0 && !selectedRoot) {
        await selectRoot(paths[0]);
      }
    } catch (err) {
      setError(`Failed to load Minecraft paths: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  const selectRoot = async (rootPath: RootPath) => {
    setSelectedRoot(rootPath);
    setLoading(true);
    try {
      const [versionsData, modsData] = await Promise.all([
        PathApi.getVersions(rootPath.path),
        PathApi.getRootMods(rootPath.path)
      ]);
      setVersions(versionsData);
      setRootMods(modsData);
    } catch (err) {
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
