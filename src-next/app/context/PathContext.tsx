"use client"
import React, { createContext, useState, useEffect, useContext } from 'react';
import { PathApi, RootPath, VersionPath } from 'app/rust-api/PathApi';

// Enhance the context interface to include version selection
interface PathContextType {
  roots: RootPath[] | null;
  versions: VersionPath[] | null;
  loading: boolean;
  error: string | null;
  selectedRoot: RootPath | null;
  selectedVersion: string | null;
  selectRoot: (root: RootPath) => void;
  selectVersion: (version: string) => void;
  refreshPaths: () => Promise<void>;
}

// Default context values
const defaultContext: PathContextType = {
  roots: null,
  versions: null,
  loading: true,
  error: null,
  selectedRoot: null,
  selectedVersion: null,
  selectRoot: () => {},
  selectVersion: () => {},
  refreshPaths: async () => {}
};

export const PathContext = createContext<PathContextType>(defaultContext);

export const PathProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [roots, setRoots] = useState<RootPath[] | null>(null);
  const [versions, setVersions] = useState<VersionPath[] | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedRoot, setSelectedRoot] = useState<RootPath | null>(null);
  const [selectedVersion, setSelectedVersion] = useState<string | null>(null);

  // Function to fetch paths
  const fetchPaths = async () => {
    try {
      setLoading(true);
      setError(null);
      
      const rootPaths = await PathApi.getMinecraftPaths();
      setRoots(rootPaths);
      
      if (rootPaths.length > 0 && selectedRoot === null) {
        setSelectedRoot(rootPaths[0]);
      }
      
      // Clear versions if no root is selected
      if (selectedRoot === null) {
        setVersions(null);
        return;
      }
      
      // Fetch versions for the selected root
      const versionPaths = await PathApi.getVersions(selectedRoot.path);
      setVersions(versionPaths);
    } catch (err) {
      console.error("Failed to fetch paths:", err);
      setError(`加载路径失败: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  // Initial fetch
  useEffect(() => {
    fetchPaths();
  }, []);

  // Fetch versions when selected root changes
  useEffect(() => {
    if (selectedRoot) {
      const fetchVersions = async () => {
        try {
          setLoading(true);
          const versionPaths = await PathApi.getVersions(selectedRoot.path);
          setVersions(versionPaths);
        } catch (err) {
          console.error("Failed to fetch versions:", err);
          setError(`加载版本失败: ${err}`);
        } finally {
          setLoading(false);
        }
      };
      
      fetchVersions();
    } else {
      setVersions(null);
    }
  }, [selectedRoot]);

  // Select root function
  const selectRoot = (root: RootPath) => {
    setSelectedRoot(root);
    // Reset selected version when root changes
    setSelectedVersion(null);
  };

  // Select version function
  const selectVersion = (version: string) => {
    setSelectedVersion(version);
  };

  // Refresh paths function
  const refreshPaths = async () => {
    await fetchPaths();
  };

  return (
    <PathContext.Provider value={{
      roots,
      versions,
      loading,
      error,
      selectedRoot,
      selectedVersion,
      selectRoot,
      selectVersion,
      refreshPaths
    }}>
      {children}
    </PathContext.Provider>
  );
};

// Custom hook for using the context
export const usePath = () => useContext(PathContext);
