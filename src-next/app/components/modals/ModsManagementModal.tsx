"use client";
import { useState, useEffect, useContext } from 'react';
import { PathApi, ModPath } from 'app/rust-api/PathApi';
import { FaFolderOpen, FaFileArchive } from 'react-icons/fa';
import { PathContext } from 'app/context/PathContext';

interface ModsManagementModalProps {
  isOpen: boolean;
  onClose: () => void;
  rootPath: string;
  versionDisplayName?: string;
}

export default function ModsManagementModal({
  isOpen,
  onClose,
  rootPath,
  versionDisplayName
}: ModsManagementModalProps) {
  const { selectedVersion: versionName } = useContext(PathContext);
  const [mods, setMods] = useState<ModPath[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const fetchMods = async () => {
      if (!isOpen || !versionName) return;
      
      setLoading(true);
      setError(null);
      
      try {
        let modsData: ModPath[];
        if (versionName === "global") {
          modsData = await PathApi.getRootMods(rootPath);
        } else {
          modsData = await PathApi.getVersionMods(rootPath, versionName);
        }
        setMods(modsData);
      } catch (err) {
        console.error("Failed to load mods:", err);
        setError(`加载模组失败: ${err}`);
      } finally {
        setLoading(false);
      }
    };

    fetchMods();
  }, [isOpen, rootPath, versionName]);

  if (!isOpen || !versionName) return null;

  const title = versionName === "global" 
    ? "通用模组" 
    : `${versionDisplayName || versionName} 模组`;

  return (
    <div className="modal modal-open">
      <div className="modal-box max-w-4xl">
        <h3 className="font-bold text-xl mb-4 flex items-center">
          <FaFileArchive className="mr-2" />
          {title}
        </h3>
        
        {loading ? (
          <div className="flex justify-center items-center p-8">
            <span className="loading loading-spinner loading-lg"></span>
            <p className="ml-2">加载模组中...</p>
          </div>
        ) : error ? (
          <div className="alert alert-error">
            <span>{error}</span>
          </div>
        ) : mods.length === 0 ? (
          <div className="text-center py-8">
            <p className="text-lg">没有找到模组</p>
            <p className="text-sm opacity-70 mt-2">
              {versionName === "global" 
                ? "通用模组目录为空" 
                : `${versionDisplayName || versionName} 没有专属模组`}
            </p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="table table-zebra w-full">
              <thead>
                <tr>
                  <th>模组名称</th>
                  <th>位置</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                {mods.map((mod, index) => (
                  <tr key={index}>
                    <td>{mod.name}</td>
                    <td>
                      <div className="flex items-center">
                        <span className="badge badge-outline">
                          {mod.location === "global" ? "通用" : mod.location}
                        </span>
                      </div>
                    </td>
                    <td>
                      <button 
                        className="btn btn-xs btn-outline"
                        onClick={() => {
                          // Open the directory containing the mod
                          const path = mod.path.substring(0, mod.path.lastIndexOf('\\'));
                          window.open(`file://${path}`, '_blank');
                        }}
                      >
                        <FaFolderOpen className="mr-1" /> 打开目录
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        <div className="modal-action">
          <button className="btn" onClick={onClose}>关闭</button>
        </div>
      </div>
    </div>
  );
}
