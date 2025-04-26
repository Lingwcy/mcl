"use client";
import React, { useState, useRef, useEffect } from 'react';
import { invoke } from '@tauri-apps/api/core';
import { open } from '@tauri-apps/plugin-dialog';
import { listen } from '@tauri-apps/api/event';

type MinecraftVersion = {
  id: string;
  type_: string;
  url: string;
  time: string;
  release_time: string;
};

type DownloadProgress = {
  version_id: string;
  progress: number;
  total: number;
  percentage: number;
};

type DownloadStatus = {
  [versionId: string]: {
    status: 'idle' | 'downloading' | 'complete' | 'error';
    progress: number;
    error?: string;
  };
};

export default function DevLaunchContext() {
  const [username, setUsername] = useState<string>('Ou_Takahiro');
  const [gameDirectory, setGameDirectory] = useState<string>('');
  const gameDirectoryRef = useRef<string>('');
  const [versionFilter, setVersionFilter] = useState<string>('');
  const [versionType, setVersionType] = useState<string>('release');
  const [versions, setVersions] = useState<MinecraftVersion[]>([]);
  const [installedVersions, setInstalledVersions] = useState<string[]>([]);
  const [selectedVersion, setSelectedVersion] = useState<string>('');
  const [loading, setLoading] = useState<boolean>(false);
  const [message, setMessage] = useState<{text: string, type: 'info' | 'error' | 'success'} | null>(null);
  const [downloadStatus, setDownloadStatus] = useState<DownloadStatus>({});
  
  // Section visibility states
  const [showSearch, setShowSearch] = useState<boolean>(false);
  const [showLaunch, setShowLaunch] = useState<boolean>(true);

  // Update the ref whenever gameDirectory changes
  useEffect(() => {
    gameDirectoryRef.current = gameDirectory;
  }, [gameDirectory]);

  // Fetch default game directory on component mount
  useEffect(() => {
    const fetchInitialData = async () => {
      try {
        const directory = await invoke<string>('get_default_game_directory');
        setGameDirectory(directory);
        gameDirectoryRef.current = directory; // Initialize the ref
        await fetchInstalledVersions(directory);
      } catch (error) {
        setMessage({
          text: `获取初始数据错误: ${error}`,
          type: 'error'
        });
      }
    };

    fetchInitialData();
    
    // Set up event listeners for download progress
    const unlisten1 = listen<DownloadProgress>('download-progress', (event) => {
      const { version_id, percentage } = event.payload;
      setDownloadStatus(prev => ({
        ...prev,
        [version_id]: {
          ...prev[version_id],
          status: 'downloading',
          progress: percentage
        }
      }));
    });

    const unlisten2 = listen<string>('download-complete', (event) => {
      const version_id = event.payload;
      setDownloadStatus(prev => ({
        ...prev,
        [version_id]: {
          ...prev[version_id],
          status: 'complete',
          progress: 100
        }
      }));
      
      // Use the ref to get the latest value
      fetchInstalledVersions(gameDirectoryRef.current);
    });

    const unlisten3 = listen<{version_id: string, error: string}>('download-error', (event) => {
      const { version_id, error } = event.payload;
      setDownloadStatus(prev => ({
        ...prev,
        [version_id]: {
          ...prev[version_id],
          status: 'error',
          error
        }
      }));
      setMessage({
        text: error,
        type: 'error'
      });
    });

    const unlisten4 = listen<string>('download-status', (event) => {
      // Final status message - can be used for overall UI updates
      setMessage({
        text: event.payload,
        type: event.payload.includes('Error') ? 'error' : 'success'
      });
      setLoading(false);
    });

    return () => {
      // Clean up listeners when component unmounts
      unlisten1.then(fn => fn());
      unlisten2.then(fn => fn());
      unlisten3.then(fn => fn());
      unlisten4.then(fn => fn());
    };
  }, []); 

  // Add a useEffect to refetch installed versions when gameDirectory changes
  useEffect(() => {
    if (gameDirectory) {
      fetchInstalledVersions(gameDirectory);
    }
  }, [gameDirectory]);

  const selectGameDirectory = async () => {
    try {
      const selected = await open({
        directory: true,
        multiple: false,
        title: '选择Minecraft游戏目录'
      });
      
      if (selected && typeof selected === 'string') {
        setGameDirectory(selected);
        setMessage({
          text: `游戏目录设置为: ${selected}`,
          type: 'success'
        });
      }
    } catch (error) {
      setMessage({
        text: `选择目录错误: ${error}`,
        type: 'error'
      });
    }
  };

  const fetchInstalledVersions = async (directory: string) => {
    try {
      const versions = await invoke<string[]>('get_installed_versions', { gameDir: directory });
      setInstalledVersions(versions);
    } catch (error) {
      setMessage({
        text: `获取已安装版本错误: ${error}`,
        type: 'error'
      });
    }
  };

  const searchVersions = async () => {
    setLoading(true);
    setMessage(null);
    try {
      const result = await invoke<MinecraftVersion[]>('search_versions', {
        versionFilter: versionFilter.trim() ? versionFilter : null,
        versionType
      });
      setVersions(result);
      setMessage({
        text: `找到 ${result.length} 个版本`,
        type: 'info'
      });
    } catch (error) {
      setMessage({
        text: `搜索版本错误: ${error}`,
        type: 'error'
      });
    } finally {
      setLoading(false);
    }
  };

  const downloadVersion = async (versionId: string) => {
    // Initialize download status
    setDownloadStatus(prev => ({
      ...prev,
      [versionId]: {
        status: 'downloading',
        progress: 0
      }
    }));
    
    setLoading(true);
    setMessage(null);
    try {
      // Use gameDirectory directly here as it's the current value
      const result = await invoke<string>('download_version', { 
        versionId, 
        gameDir: gameDirectory 
      });
      
      // Just show that download has started, progress will be updated via events
      setMessage({
        text: result,
        type: 'info'
      });
      
      // Don't refresh versions yet - will do that when download completes
    } catch (error) {
      setMessage({
        text: `${error}`,
        type: 'error'
      });
      setDownloadStatus(prev => ({
        ...prev,
        [versionId]: {
          ...prev[versionId],
          status: 'error',
          error: `${error}`
        }
      }));
      setLoading(false);
    }
  };

  const launchGame = async () => {
    if (!selectedVersion) {
      setMessage({
        text: '请选择要启动的版本',
        type: 'error'
      });
      return;
    }

    setLoading(true);
    setMessage(null);
    try {
      const result = await invoke<string>('launch_game', {
        username,
        versionId: selectedVersion,
        gameDir: gameDirectory
      });
      setMessage({
        text: result,
        type: 'success'
      });
    } catch (error) {
      setMessage({
        text: `${error}`,
        type: 'error'
      });
    } finally {
      setLoading(false);
    }
  };

  // Helper function to render download status
  const renderDownloadStatus = (versionId: string) => {
    const status = downloadStatus[versionId];
    if (!status) return null;

    if (status.status === 'downloading') {
      return (
        <div className="mt-1">
          <progress 
            className="progress progress-primary w-full" 
            value={status.progress} 
            max="100"
          ></progress>
          <span className="text-xs">{status.progress}%</span>
        </div>
      );
    }
    
    return null;
  };

  return (
    <div className="card bg-base-200 shadow-xl text-base-content h-full flex flex-col">
      <div className="card-body p-3 flex flex-col overflow-hidden h-full">
        <h2 className="card-title text-base-content text-center md:text-left mb-2">资源下载测试</h2>
        
        <div className="flex flex-col gap-2 overflow-auto flex-grow">
          {/* Game directory selector */}
          <div className="card bg-base-300">
            <div className="card-body p-3">
              <h3 className="card-title text-base-content text-md">游戏目录</h3>
              <div className="flex flex-col md:flex-row gap-2">
                <input
                  type="text"
                  value={gameDirectory}
                  onChange={(e) => setGameDirectory(e.target.value)}
                  className="input input-bordered input-sm w-full"
                  placeholder="游戏目录路径"
                  readOnly
                />
                <button
                  onClick={selectGameDirectory}
                  className="btn btn-primary btn-sm"
                >
                  浏览
                </button>
              </div>
            </div>
          </div>

          {/* Username input */}
          <div className="form-control">
            <div className="flex flex-row items-center gap-2">
              <label className="label-text whitespace-nowrap">用户名:</label>
              <input
                type="text"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                className="input input-bordered input-sm w-full"
                placeholder="用户名"
              />
            </div>
          </div>
          
          {/* Search section */}
          <div className="card bg-base-300">
            <div className="card-body p-3">
              <div className="flex justify-between items-center mb-1">
                <h3 className="card-title text-md">搜索资源</h3>
                <button 
                  className="btn btn-xs btn-ghost" 
                  onClick={() => setShowSearch(!showSearch)}
                >
                  {showSearch ? '收起' : '展开'}
                </button>
              </div>
              
              {showSearch && (
                <>
                  <div className="flex flex-col md:flex-row gap-2 mb-2">
                    <input
                      type="text"
                      value={versionFilter}
                      onChange={(e) => setVersionFilter(e.target.value)}
                      className="input input-bordered input-sm w-full"
                      placeholder="版本过滤器 (例如 1.19)"
                    />
                    <select
                      value={versionType}
                      onChange={(e) => setVersionType(e.target.value)}
                      className="select select-bordered select-sm w-full md:w-auto"
                    >
                      <option value="release">正式版</option>
                      <option value="snapshot">快照版</option>
                      <option value="old_beta">旧测试版</option>
                      <option value="old_alpha">旧内测版</option>
                    </select>
                    <button
                      onClick={searchVersions}
                      disabled={loading}
                      className="btn btn-primary btn-sm"
                    >
                      {loading ? '搜索中...' : '搜索'}
                    </button>
                  </div>

                  {/* Search results */}
                  {versions.length > 0 && (
                    <div className="max-h-40 overflow-auto">
                      <table className="table table-zebra table-xs">
                        <thead className="sticky top-0 bg-base-300 z-10">
                          <tr>
                            <th>版本</th>
                            <th>类型</th>
                            <th className="hidden md:table-cell">发布日期</th>
                            <th>操作</th>
                          </tr>
                        </thead>
                        <tbody>
                          {versions.map((version) => (
                            <tr key={version.id}>
                              <td>{version.id}</td>
                              <td>{version.type_}</td>
                              <td className="hidden md:table-cell">{new Date(version.release_time).toLocaleDateString()}</td>
                              <td>
                                <div>
                                  <button
                                    onClick={() => downloadVersion(version.id)}
                                    disabled={loading || installedVersions.includes(version.id) || 
                                      (downloadStatus[version.id]?.status === 'downloading')}
                                    className={`btn btn-xs ${
                                      installedVersions.includes(version.id) ? 'btn-success' : 'btn-primary'
                                    }`}
                                  >
                                    {installedVersions.includes(version.id) ? '已安装' : 
                                    downloadStatus[version.id]?.status === 'downloading' ? '下载中...' : '下载'}
                                  </button>
                                  {renderDownloadStatus(version.id)}
                                </div>
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  )}
                </>
              )}
            </div>
          </div>

          {/* Launch section */}
          <div className="card bg-base-300 flex-grow">
            <div className="card-body p-3">
              <div className="flex justify-between items-center mb-1">
                <h3 className="card-title text-md">启动游戏</h3>
                <button 
                  className="btn btn-xs btn-ghost" 
                  onClick={() => setShowLaunch(!showLaunch)}
                >
                  {showLaunch ? '收起' : '展开'}
                </button>
              </div>
              
              {showLaunch && (
                <>
                  {installedVersions.length > 0 ? (
                    <div className="flex flex-col gap-2">
                      <div className="flex flex-row items-center gap-2">
                        <label className="label-text whitespace-nowrap">选择版本:</label>
                        <select
                          value={selectedVersion}
                          onChange={(e) => setSelectedVersion(e.target.value)}
                          className="select select-bordered select-sm w-full"
                        >
                          <option value="">选择一个版本</option>
                          {installedVersions.map(version => (
                            <option key={version} value={version}>{version}</option>
                          ))}
                        </select>
                      </div>
                      <button
                        onClick={launchGame}
                        disabled={loading || !selectedVersion}
                        className="btn btn-secondary btn-sm"
                      >
                        {loading ? '启动中...' : '启动游戏'}
                      </button>
                    </div>
                  ) : (
                    <div className="alert alert-sm p-2">未安装任何版本，请先下载一个版本。</div>
                  )}
                </>
              )}
            </div>
          </div>
        </div>

        {/* Status message */}
        {message && (
          <div className={`alert text-sm p-2 mt-2 ${
            message.type === 'error' ? 'alert-error' : 
            message.type === 'success' ? 'alert-success' : 
            'alert-info'
          }`}>
            {message.text}
          </div>
        )}
      </div>
    </div>
  );
}
