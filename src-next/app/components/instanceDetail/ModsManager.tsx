import { useState, useEffect, useContext } from 'react';
import { PathApi, ModPath } from 'app/rust-api/PathApi';
import { PathContext } from 'app/context/PathContext';

export default function ModsManagementContent() {
  const { 
    roots, 
    loading: contextLoading, 
    error: contextError,
    selectedRoot,
    selectedVersion: versionName
  } = useContext(PathContext);
  
  const [searchTerm, setSearchTerm] = useState('');
  const [versionMods, setVersionMods] = useState<ModPath[]>([]);
  const [globalMods, setGlobalMods] = useState<ModPath[]>([]);
  const [filteredMods, setFilteredMods] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [hoveredModId, setHoveredModId] = useState<string | null>(null);

  // Fetch mods from API
  useEffect(() => {
    const fetchMods = async () => {
      if (!selectedRoot || !versionName) return;
      
      setLoading(true);
      setError(null);
      
      try {
        const rootPath = selectedRoot.path;
        
        // Always fetch global mods
        const globalModsData = await PathApi.getRootMods(rootPath);
        setGlobalMods(globalModsData);
        
        if (versionName === "global") {
          // If we're in global mode, there are no version-specific mods
          setVersionMods([]);
        } else {
          // Fetch version-specific mods
          const versionModsData = await PathApi.getVersionMods(rootPath, versionName);
          setVersionMods(versionModsData);
        }
      } catch (err) {
        console.error("Failed to load mods:", err);
        setError(`加载模组失败: ${err}`);
      } finally {
        setLoading(false);
      }
    };

    fetchMods();
  }, [selectedRoot, versionName]);

  // Transform mods data for UI and apply search filter
  useEffect(() => {
    // Combine both types of mods
    const allMods = [...versionMods, ...globalMods];
    if (allMods.length === 0) return;
    
    const transformed = allMods.map((mod) => ({
      id: mod.path, // Use path as unique ID
      name: mod.name,
      icon: 'https://optifine.net/favicon.ico', // Default icon
      tags: [mod.location === "global" ? "通用" : mod.location], // Use location as a tag
      version: '',
      description: '',
      path: mod.path,
      isGlobal: mod.location === "global"
    }));
    
    const filtered = transformed.filter(mod => 
      mod.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
      (mod.description && mod.description.toLowerCase().includes(searchTerm.toLowerCase())) ||
      mod.tags.some(tag => tag.toLowerCase().includes(searchTerm.toLowerCase()))
    );
    
    // Sort to show version-specific mods first, then global mods
    filtered.sort((a, b) => {
      if (a.isGlobal === b.isGlobal) return 0;
      return a.isGlobal ? 1 : -1;
    });
    
    setFilteredMods(filtered);
  }, [versionMods, globalMods, searchTerm]);

  const handleDeleteMod = (id: string) => {
    // Placeholder for delete functionality
    console.log(`Delete mod with id: ${id}`);
  };

  const handleOpenFolder = (path: string) => {
    // Open the directory containing the mod
    const folderPath = path.substring(0, path.lastIndexOf('\\'));
    window.open(`file://${folderPath}`, '_blank');
  };

  // Use context loading state if we're still loading roots
  if (contextLoading) {
    return (
      <div className="flex justify-center items-center p-8 h-full">
        <span className="loading loading-spinner loading-lg"></span>
        <p className="ml-2">加载根目录中...</p>
      </div>
    );
  }

  // Show error from context if there is one
  if (contextError) {
    return (
      <div className="alert alert-error m-4">
        <span>{contextError}</span>
      </div>
    );
  }

  if (!selectedRoot) {
    return (
      <div className="alert alert-warning m-4">
        <span>请先选择一个 Minecraft 目录</span>
      </div>
    );
  }

  if (!versionName) {
    return (
      <div className="alert alert-info m-4">
        <span>请先选择一个游戏版本</span>
      </div>
    );
  }

  if (loading) {
    return (
      <div className="flex justify-center items-center p-8 h-full">
        <span className="loading loading-spinner loading-lg"></span>
        <p className="ml-2">加载模组中...</p>
      </div>
    );
  }

  if (error) {
    return (
      <div className="alert alert-error m-4">
        <span>{error}</span>
      </div>
    );
  }

  // Calculate if we should show section headers
  const hasVersionMods = filteredMods.some(mod => !mod.isGlobal);
  const hasGlobalMods = filteredMods.some(mod => mod.isGlobal);
  const showSectionHeaders = hasVersionMods && hasGlobalMods;

  const versionDisplayName = versionName === "global" ? "通用" : versionName;

  return (
    <div className="text-base-content container mx-auto p-4 bg-base-300 h-full flex flex-col">
      <div className="flex justify-between items-center mb-4">
        <div className="form-control w-full">
          <input
            type="text"
            placeholder="搜索模组..."
            className="input w-full"
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
          />
        </div>
      </div>
      
      <div className="flex-1 bg-base-300 transition-opacity duration-300 ease-in-out overflow-y-auto max-h-[calc(100vh-150px)]">
        {/* Version-specific mods section header if needed */}
        {showSectionHeaders && hasVersionMods && (
          <div className="bg-base-200 py-2 px-4 text-sm font-medium text-base-content mb-2">
            {versionDisplayName} 
          </div>
        )}
        
        {/* Version-specific mods */}
        {filteredMods.filter(mod => !mod.isGlobal).map((mod) => (
          <div 
            key={mod.id} 
            className="card pl-2 card-side bg-base-300 rounded-none relative"
            onMouseEnter={() => setHoveredModId(mod.id)}
            onMouseLeave={() => setHoveredModId(null)}
          >
            <figure className="p-2 w-14 flex-shrink-0">
              <img
                src={mod.icon}
                className="w-full h-auto rounded"
                alt={mod.name} 
              />
            </figure>
            <div className="card-body py-2 px-3 flex flex-row items-center">
              <div className="flex-1">
                <h2 className="card-title text-sm text-base-content truncate">
                  {mod.name} {mod.version && <span className="text-xs text-gray-400 font-normal">| {mod.version}</span>}
                </h2>
                {mod.description && <p className="text-xs text-gray-500 font-light mt-1">{mod.description}</p>}
                <div className="flex flex-wrap gap-2 mt-1">
                  {mod.tags.map((tag: string, idx: number) => (
                    <span 
                      key={idx} 
                      className="px-2 py-1 bg-base-200 text-xs rounded-lg text-gray-600"
                      style={{display: 'inline-block'}}
                    >
                      {tag}
                    </span>
                  ))}
                </div>
              </div>
              
              <div className={`flex gap-2 transition-all duration-300 ease-in-out ${hoveredModId === mod.id ? 'opacity-100 translate-x-0' : 'opacity-0 translate-x-5 pointer-events-none'}`}>
                <button 
                  className="btn btn-sm btn-circle" 
                  onClick={() => handleDeleteMod(mod.id)}
                  title="删除模组"
                >
                    <svg xmlns="http://www.w3.org/2000/svg" className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                    </svg>
                </button>
                <button 
                  className="btn btn-sm btn-circle"
                  onClick={() => handleOpenFolder(mod.path)}
                  title="打开文件夹"
                >
                  <svg xmlns="http://www.w3.org/2000/svg" className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 19a2 2 0 01-2-2V7a2 2 0 012-2h4l2 2h4a2 2 0 012 2v1M5 19h14a2 2 0 002-2v-5a2 2 0 00-2-2H9a2 2 0 00-2 2v5a2 2 0 01-2 2z" />
                  </svg>
                </button>
              </div>
            </div>
          </div>
        ))}
        
        {/* Global mods section header if needed */}
        {showSectionHeaders && hasGlobalMods && (
          <div className="bg-base-200 py-2 px-4 text-sm font-medium text-base-content mb-2 mt-3">
            通用mods
          </div>
        )}
        
        {/* Global mods */}
        {filteredMods.filter(mod => mod.isGlobal).map((mod) => (
          <div 
            key={mod.id} 
            className="card pl-2 card-side bg-base-300 rounded-none relative"
            onMouseEnter={() => setHoveredModId(mod.id)}
            onMouseLeave={() => setHoveredModId(null)}
          >
            <figure className="p-2 w-14 flex-shrink-0">
              <img
                src={mod.icon}
                className="w-full h-auto rounded"
                alt={mod.name} 
              />
            </figure>
            <div className="card-body py-2 px-3 flex flex-row items-center">
              <div className="flex-1">
                <h2 className="card-title text-sm text-base-content truncate">
                  {mod.name} {mod.version && <span className="text-xs text-gray-400 font-normal">| {mod.version}</span>}
                </h2>
                {mod.description && <p className="text-xs text-gray-500 font-light mt-1">{mod.description}</p>}
                <div className="flex flex-wrap gap-2 mt-1">
                  {mod.tags.map((tag: string, idx: number) => (
                    <span 
                      key={idx} 
                      className="px-2 py-1 bg-base-200 text-xs rounded-lg text-gray-600"
                      style={{display: 'inline-block'}}
                    >
                      {tag}
                    </span>
                  ))}
                </div>
              </div>
              
              <div className={`flex gap-2 transition-all duration-300 ease-in-out ${hoveredModId === mod.id ? 'opacity-100 translate-x-0' : 'opacity-0 translate-x-5 pointer-events-none'}`}>
                <button 
                  className="btn btn-sm btn-circle" 
                  onClick={() => handleDeleteMod(mod.id)}
                  title="删除模组"
                >
                    <svg xmlns="http://www.w3.org/2000/svg" className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                    </svg>
                </button>
                <button 
                  className="btn btn-sm btn-circle"
                  onClick={() => handleOpenFolder(mod.path)}
                  title="打开文件夹"
                >
                  <svg xmlns="http://www.w3.org/2000/svg" className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 19a2 2 0 01-2-2V7a2 2 0 012-2h4l2 2h4a2 2 0 012 2v1M5 19h14a2 2 0 002-2v-5a2 2 0 00-2-2H9a2 2 0 00-2 2v5a2 2 0 01-2 2z" />
                  </svg>
                </button>
              </div>
            </div>
          </div>
        ))}
        
        {filteredMods.length === 0 && (
          <div className="text-center py-10 text-base-content">
            <h3 className="text-xl font-bold">没有找到模组</h3>
            <p className="text-sm mt-2">尝试使用其他关键词进行搜索</p>
          </div>
        )}
      </div>
    </div>
  );
}