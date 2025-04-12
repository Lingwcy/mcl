import { useState } from 'react';
import { FiFolder, FiSettings } from "react-icons/fi";

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
    // Available folders
    const folders: FolderItem[] = [
        { id: 'default', name: '默认文件夹' },
        { id: 'modded', name: '官方文件夹' },
        { id: 'server', name: 'MiaoClient' },
    ];

    // State to track currently selected folder
    const [selectedFolder, setSelectedFolder] = useState(folderId);
    // State to control file management modal
    const [isModalOpen, setIsModalOpen] = useState(false);

    // Generate instances with folder assignments
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