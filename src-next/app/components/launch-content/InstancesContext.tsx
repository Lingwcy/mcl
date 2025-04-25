import { useState } from 'react';
import { FiFolder, FiSettings } from "react-icons/fi";
import { useContext } from 'react';
import { PathContext } from 'app/context/PathContext';
import { MdOutlineSelectAll } from "react-icons/md";
import SelectGame from '../other/SelectGame';


interface MinecraftInstance {
    id: string;
    name: string;
    loader: 'Fabric' | 'Forge' | 'Vanilla' | 'Quilt';
    version: string;
    folderId: string;
}

interface InstanceContextProps {
    folderId?: string;
}

interface FolderItem {
    id: string;
    name: string;
}

export default function InstanceContext({ folderId = 'default' }: InstanceContextProps) {
    const {
        roots,
    } = useContext(PathContext);
    const [isOpen, setIsOpen] = useState(false);
    const OpenSelectGame = () => setIsOpen(true);
    
    if (roots?.length == 0) {
        return (
            <div className="h-full flex items-center justify-center">
              <div className="text-center">
                <h2 className="text-2xl font-bold mb-2">你还没有选择任何Minecraft游戏文件夹</h2>
                <button className='btn btn-soft w-50' onClick={OpenSelectGame}>
                    <MdOutlineSelectAll />
                    选择游戏
                </button>
              </div>
              <SelectGame isOpen={isOpen} setIsOpen={setIsOpen} />
            </div>
          );
    }
    const folders: FolderItem[] = [
        { id: 'default', name: '/home/ling/桌面/mc' },
        { id: 'modded', name: '官方文件夹' },
        { id: 'server', name: 'MiaoClient' },
    ];
    const [selectedFolder, setSelectedFolder] = useState(folderId);
    const [isModalOpen, setIsModalOpen] = useState(false);
    const [instances] = useState<MinecraftInstance[]>(() => {
        const loaders = ['Fabric'] as const;
        const versions = ['1.20.1', '1.19.4', '1.18.2', '1.21.5', '1.16.5'];
        const folderIds = ['default', 'modded', 'server'];

        return Array.from({ length: 20 }, (_, i) => ({
            id: `instance-${i + 1}`,
            name: `Minecraft Server ${i + 1}`,
            loader: loaders[i % loaders.length],
            version: versions[i % versions.length],
            folderId: folderIds[i % folderIds.length],
        }));
    });

    const filteredInstances = instances.filter(instance => instance.folderId === selectedFolder);

    return (
        <div className="h-full flex flex-col overflow-hidden">
            <div className="flex justify-between items-center mt-3">
                <ul className="menu menu-horizontal bg-base-300 rounded-box w-full text-base-content flex">
                    <div className="flex-1 flex">
                        {folders.map((folder) => (
                            <li key={folder.id}>
                                <a
                                    onClick={() => setSelectedFolder(folder.id)}
                                    className={selectedFolder === folder.id ? 'active' : ''}
                                >
                                    <FiFolder className="mr-1" /> {folder.name}
                                </a>
                            </li>
                        ))}
                    </div>
                    <li className="ml-auto">
                        <a onClick={() => setIsModalOpen(true)}>
                            <FiSettings />
                        </a>
                    </li>
                </ul>

            </div>

    
            {isModalOpen && (
                <div className="modal modal-open text-base-content">
                    <div className="modal-box">
                        <h3 className="font-bold text-lg">文件管理</h3>
                        <p className="py-4">在这里管理您的Minecraft实例文件夹</p>
                        <div className="modal-action">
                            <button
                                className="btn"
                                onClick={() => setIsModalOpen(false)}
                            >
                                关闭
                            </button>
                        </div>
                    </div>
                </div>
            )}

            <div className="flex-1 overflow-y-auto p-4 grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                {filteredInstances.map((instance) => (
                    <div key={instance.id} className="card card-side bg-base-100 shadow-sm h-30">
                        <figure className="p-4 w-24">
                            <img
                                src={`bands/${instance.loader.toLowerCase()}.png`}
                                className="w-full h-auto"
                                alt={instance.loader} />
                        </figure>
                        <div className="card-body py-4 pr-4 pl-0">
                            <h2 className="card-title text-md text-base-content">{instance.name}</h2>
                            <p className="text-base-content">{instance.loader} Minecraft {instance.version}</p>
                            <div className="card-actions justify-end mt-2">
                                <button className="btn btn-soft btn-primary btn-xs">选定</button>
                                <button className="btn btn-soft btn-primary btn-xs">管理</button>
                                <button className="btn btn-soft btn-error btn-xs">删除</button>
                            </div>
                        </div>
                    </div>
                ))}
            </div>
        </div>
    );
}