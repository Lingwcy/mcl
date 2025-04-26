"use client";
import { useState, useEffect, useRef } from 'react';
import { FiFolder, FiSettings } from "react-icons/fi";
import { useContext } from 'react';
import { PathContext } from 'app/context/PathContext';
import { MdOutlineSelectAll } from "react-icons/md";
import SelectGame from '../other/SelectGame';
import { RootPath } from 'app/rust-api/PathApi';
import ModsManagementModal from '../modals/ModsManagementModal';
import { useUI } from '../../context/UIContext';

interface InstanceContextProps {
    folderId?: string; // Keep this for backward compatibility
}

export default function InstanceContext({}: InstanceContextProps) {
    const {
        roots,
        versions,
        loading,
        error,
        selectRoot,
        refreshPaths
    } = useContext(PathContext);
    
    const { uiState } = useUI();
    
    const [isOpen, setIsOpen] = useState(false);
    const [selectedFolder, setSelectedFolder] = useState<string | undefined>(undefined);
    const [isModalOpen, setIsModalOpen] = useState(false);
    const [isModsModalOpen, setIsModsModalOpen] = useState(false);
    const [selectedVersion, setSelectedVersion] = useState<string | null>(null);
    const [selectedVersionName, setSelectedVersionName] = useState<string | null>(null);
    const [isSidebarMinimized, setIsSidebarMinimized] = useState(true);
    const [textVisible, setTextVisible] = useState(!isSidebarMinimized);
    const [isContentTransitioning, setIsContentTransitioning] = useState(false);
    const [contentOpacity, setContentOpacity] = useState(1);
    
    // Use a ref to track if we've already initialized
    const initializedRef = useRef(false);
    
    // Add effect to handle text visibility with delay
    useEffect(() => {
        if (isSidebarMinimized) {
            setTextVisible(false);
        } else {
            // Delay showing text until sidebar expands
            const timer = setTimeout(() => setTextVisible(true), 180);
            return () => clearTimeout(timer);
        }
    }, [isSidebarMinimized]);
    
    // Initialize selectedFolder when roots are loaded - but only once
    useEffect(() => {
        if (roots && roots.length > 0 && !selectedFolder && !initializedRef.current) {
            console.log("Setting initial selected folder:", roots[0].path);
            setSelectedFolder(roots[0].path);
            selectRoot(roots[0]);
            initializedRef.current = true;
        }
    }, [roots, selectedFolder, selectRoot]);

    const OpenSelectGame = () => setIsOpen(true);
    
    const handleFolderSelect = (root: RootPath) => {
        if (selectedFolder === root.path) return; // Prevent unnecessary transitions
        
        console.log("Folder selected:", root);
        
        // Start the transition effect
        setIsContentTransitioning(true);
        setContentOpacity(0);
        
        // Short delay before actually changing the data
        setTimeout(() => {
            setSelectedFolder(root.path);
            selectRoot(root);
            
            // Fade back in
            setTimeout(() => {
                setContentOpacity(50);
                setTimeout(() => {
                    setIsContentTransitioning(false);
                }, 300); 
            }, 50);
        }, 200);
    };
    
    // After a successful addition, we need to ensure the UI updates
    const handleSuccessfulAddition = async () => {
        console.log("Handling successful directory addition");
        await refreshPaths();
    };
    
    const handleManageMods = (versionName: string) => {
        setSelectedVersion(versionName);
        setSelectedVersionName(versionName);
        setIsModsModalOpen(true);
    };
    
    // If no Minecraft folders are available
    if (!roots || roots.length === 0) {
        return (
            <div className="h-full flex items-center justify-center">
              <div className="text-center">
                <h2 className="text-2xl font-bold mb-2">您还没有托管任何Minecraft游戏文件夹</h2>
                <p className="text-xs">选择现有的客户端文件夹或者下载一个新的客户端！</p>
                <button className="btn btn-primary mt-4" onClick={OpenSelectGame}>
                  <MdOutlineSelectAll className="mr-2" /> 选择游戏文件夹
                </button>
              </div>
              <SelectGame 
                isOpen={isOpen} 
                setIsOpen={setIsOpen} 
                onSuccessfulSelect={handleSuccessfulAddition} 
              />
            </div>
        );
    }

    // Loading state - only show this for initial load, not transitions
    if (loading && !isContentTransitioning) {
        return (
            <div className="h-full flex items-center justify-center">
                <span className="loading loading-spinner loading-lg"></span>
                <p className="ml-2">加载中...</p>
            </div>
        );
    }

    // Error state
    if (error) {
        return (
            <div className="h-full flex items-center justify-center text-error">
                <div className="text-center">
                    <h2 className="text-2xl font-bold mb-2">发生错误</h2>
                    <p>{error}</p>
                    <button className="btn btn-primary mt-4" onClick={refreshPaths}>
                        重试
                    </button>
                </div>
            </div>
        );
    }

    return (
        <div className="h-full flex overflow-hidden">
            <div className={`bg-base-200 transition-all duration-200 ease-in-out ${isSidebarMinimized ? 'w-15' : 'w-34'} overflow-hidden`}>
                <div className="h-full w-full flex flex-col">
                    <div className="flex justify-end p-2">
                        <button 
                            onClick={() => setIsSidebarMinimized(!isSidebarMinimized)}
                            className="btn btn-sm btn-ghost"
                        >
                            {isSidebarMinimized ? 
                                <svg xmlns="http://www.w3.org/2000/svg" className="h-4 w-4 text-base-content" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 5l7 7-7 7M5 5l7 7-7 7" />
                                </svg> : 
                                <svg xmlns="http://www.w3.org/2000/svg" className="h-4 w-4 text-base-content" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M11 19l-7-7 7-7m8 14l-7-7 7-7" />
                                </svg>
                            }
                        </button>
                    </div>
                    <ul className="menu bg-base-200 w-full text-base-content flex-1">
                        {roots.map((root) => (
                            <li key={root.path}>
                                <a 
                                    className={`transition-all duration-300 hover:bg-base-300 ${
                                        selectedFolder === root.path 
                                            ? 'active bg-opacity-20 text-primary font-medium' 
                                            : ''
                                    } ${isSidebarMinimized ? 'justify-center' : ''}`}
                                    onClick={() => handleFolderSelect(root)}
                                >
                                    <span className={`flex items-center ${isSidebarMinimized ? 'justify-center' : ''} overflow-hidden`}>
                                        <FiFolder className={isSidebarMinimized ? "m-0" : "mr-2"} /> 
                                        {!isSidebarMinimized && textVisible && 
                                            <span className="transition-opacity duration-150 whitespace-nowrap">
                                                {root.display_name}
                                            </span>
                                        }
                                    </span>
                                </a>
                            </li>
                        ))}
                        <li className="mt-auto">
                            <a 
                                className={`transition-all duration-300 hover:bg-base-300 border-l-4 border-transparent ${isSidebarMinimized ? 'justify-center' : ''}`}
                                onClick={() => setIsModalOpen(true)}
                            >
                                <span className={`flex items-center ${isSidebarMinimized ? 'justify-center' : ''} overflow-hidden`}>
                                    <FiSettings className={isSidebarMinimized ? "m-0" : "mr-2"} />
                                    {!isSidebarMinimized && textVisible && 
                                        <span className="transition-opacity duration-150 whitespace-nowrap">设置</span>
                                    }
                                </span>
                            </a>
                        </li>
                    </ul>
                </div>
            </div>
            
            <div 
                className="flex-1 overflow-y-auto grid grid-cols-1 bg-base-300 transition-opacity duration-300 ease-in-out"
                style={{ opacity: contentOpacity }}
            >
                {versions && versions.length > 0 ? versions.map((version) => (
                    <div key={version.name} className="card pl-2 card-side bg-base-300 shadow-sm min-h-[70px] rounded-none">
                        <figure className="p-2 w-14 flex-shrink-0">
                            <img
                                src="/bands/fabric.png"
                                className="w-full h-auto"
                                alt="Vanilla" />
                        </figure>
                        <div className="card-body py-2 px-3 flex flex-row items-center">
                            <div className="flex-1">
                                <h2 className="card-title text-sm text-base-content truncate">{version.name}</h2>
                                <p className="text-xs text-base-content/70">{version.mod_count} mods</p>
                            </div>
                            <div className="flex items-center">
                                <button 
                                    className="btn btn-circle btn-ghost btn-sm"
                                    onClick={() => handleManageMods(version.name)}
                                >
                                    <FiSettings className="text-base-content" />
                                </button>
                            </div>
                        </div>
                    </div>
                )) : (
                    <div className="col-span-3 text-center py-10 text-base-content">
                        <h3 className="text-xl font-bold">没有找到版本</h3>
                        <p className="text-sm mt-2">在这个Minecraft文件夹中没有找到任何版本</p>
                    </div>
                )}
            </div>
            
            <SelectGame 
                isOpen={isOpen} 
                setIsOpen={setIsOpen} 
                onSuccessfulSelect={handleSuccessfulAddition} 
            />
            
            {selectedFolder && selectedVersion && (
                <ModsManagementModal
                    isOpen={isModsModalOpen}
                    onClose={() => setIsModsModalOpen(false)}
                    rootPath={selectedFolder}
                    versionName={selectedVersion}
                    versionDisplayName={selectedVersionName || undefined}
                />
            )}
        </div>
    );
}