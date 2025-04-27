import { useState, useEffect, useContext } from 'react';
import { PathApi, ModPath } from 'app/rust-api/PathApi';
import { PathContext } from 'app/context/PathContext';

export default function ModsManagementContent() {
  const {
    loading: contextLoading,
    error: contextError,
    selectedRoot,
    selectedVersion: versionName
  } = useContext(PathContext);

  const [searchTerm, setSearchTerm] = useState('');
  const [versionMods, setVersionMods] = useState<ModPath[]>([]);
  const [filteredMods, setFilteredMods] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [hoveredModId, setHoveredModId] = useState<string | null>(null);

  // 获取模组数据
  useEffect(() => {
    const fetchMods = async () => {
      if (!selectedRoot || !versionName) return;

      setLoading(true);
      setError(null);

      try {
        const rootPath = selectedRoot.path;
        const versionModsData = await PathApi.getVersionMods(rootPath, versionName);
        setVersionMods(versionModsData);
      } catch (err) {
        setError(`加载模组失败: ${err}`);
      } finally {
        setLoading(false);
      }
    };

    fetchMods();
  }, [selectedRoot, versionName]);

  // 筛选渲染数据
  useEffect(() => {
    const allMods = [...versionMods];
    if (allMods.length === 0) return;

    const transformed = allMods.map((mod) => ({
      id: mod.path,
      name: mod.name,
      icon: 'bands/fabric.png',
      tags: [mod.location, "mcl", "ling", "miaotownVI"], 
      version: '版本号',
      description: '这是一个默认介绍',
      path: mod.path,
    }));

    const filtered = transformed.filter(mod =>
      mod.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
      (mod.description && mod.description.toLowerCase().includes(searchTerm.toLowerCase())) ||
      mod.tags.some(tag => tag.toLowerCase().includes(searchTerm.toLowerCase()))
    );

    setFilteredMods(filtered);
  }, [versionMods, searchTerm]);


  // 删除模组
  const handleDeleteMod = (id: string) => {
    console.log(`Delete mod with id: ${id}`);
  };

  // 打开模组所在文件夹
  const handleOpenFolder = (path: string) => {
    // Open the directory containing the mod
    const folderPath = path.substring(0, path.lastIndexOf('\\'));
    window.open(`file://${folderPath}`, '_blank');
  };

  // 加载状态
  if (contextLoading) {
    return (
      <div className="flex justify-center items-center p-8 h-full">
        <span className="loading loading-spinner loading-lg"></span>
        <p className="ml-2">加载根目录中...</p>
      </div>
    );
  }

  // 错误状态
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

  return (
    <div className="text-base-content container mx-auto p-2 sm:p-4 bg-base-300 h-full flex flex-col">
      {/* 搜索框 */}
      <div className="flex justify-between items-center mb-2 sm:mb-4">
        <div className="form-control w-full">
          <input
            type="text"
            placeholder="搜索模组..."
            className="input input-sm sm:input-md w-full"
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
          />
        </div>
      </div>
      {/* mods表单 */}
      <div className="flex-1 bg-base-300 transition-opacity duration-300 ease-in-out overflow-y-auto">
        {filteredMods.filter(mod => !mod.isGlobal).map((mod) => (
          <div
            key={mod.id}
            className="card pl-1 sm:pl-2 bg-base-300 card-side rounded-none mb-1"
            onMouseEnter={() => setHoveredModId(mod.id)}
            onMouseLeave={() => setHoveredModId(null)}
          >
            <figure className=" p-1 sm:p-2 w-10 sm:w-14 flex-shrink-0">
              <img
                src={mod.icon}
                className="w-full h-auto rounded"
                alt={mod.name}
              />
            </figure>
            <div className="card-body py-1 sm:py-2 px-2 sm:px-3 flex flex-row items-center overflow-hidden">
              <div className="flex-1 min-w-0 max-w-full">
                <h2 className=" card-title text-xs sm:text-sm text-base-content truncate">
                  {mod.name} {mod.version && <span className="badge-info badge badge-xs"> {mod.version}</span>}
                </h2>
                {mod.description && <p className="text-xs text-gray-500 font-light mt-0.5 sm:mt-1 truncate">{mod.description}</p>}
                <div className=" flex flex-wrap gap-1 sm:gap-2 mt-0.5 sm:mt-1 overflow-hidden">
                  {mod.tags.slice(0, 3).map((tag: string, idx: number) => (
                    <span
                      key={idx}
                      className="px-1.5 py-0.1 bg-base-200 text-[0.65rem] sm:text-xs rounded-md text-gray-600 whitespace-nowrap"
                    >
                      {tag}
                    </span>
                  ))}
                  {mod.tags.length > 3 && (
                    <span className="px-1.5 py-0.1 bg-base-200 text-[0.65rem] sm:text-xs rounded-md text-gray-600">
                      +{mod.tags.length - 3}
                    </span>
                  )}
                </div>
              </div>

              <div className={` flex gap-1 sm:gap-2 transition-all duration-300 ease-in-out ${hoveredModId === mod.id ? 'opacity-100 translate-x-0' : 'opacity-0 translate-x-5 pointer-events-none'}`}>
                <button
                  className="btn btn-xs sm:btn-sm btn-circle"
                  onClick={() => handleDeleteMod(mod.id)}
                  title="删除模组"
                >
                  <svg xmlns="http://www.w3.org/2000/svg" className="h-3 w-3 sm:h-5 sm:w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                  </svg>
                </button>
                <button
                  className="btn btn-xs sm:btn-sm btn-circle"
                  onClick={() => handleOpenFolder(mod.path)}
                  title="打开文件夹"
                >
                  <svg xmlns="http://www.w3.org/2000/svg" className="h-3 w-3 sm:h-5 sm:w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
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