import React from 'react';
import { IoClose, IoRemove } from "react-icons/io5";
import { IoIosRocket } from "react-icons/io";
import { FaDownload } from "react-icons/fa6";
import { RiSettings6Fill } from "react-icons/ri";
import { TbAppsFilled } from "react-icons/tb";
import ThemeSwitcher from './theme-switcher';
// 导入 Tauri 窗口 API
import { Window } from '@tauri-apps/api/window';
const appWindow = new Window('main');
interface TopNavBarProps {
  activeTab: string | null;
  onNavClick: (tab: string) => void;
}

export default function TopNavBar({ activeTab, onNavClick }: TopNavBarProps) {
    // 处理最小化窗口
    const handleMinimize = async () => {
        await appWindow.minimize();
    };
    
    // 处理关闭窗口
    const handleClose = async () => {
        await appWindow.close();
    };
    
    return (
        <div className="navbar bg-base-100 shadow-sm min-h-10 h-10" data-tauri-drag-region>
            <div className="navbar-start" data-tauri-drag-region>
                <a className="btn btn-ghost text-xl text-base-content" data-tauri-drag-region>MCL</a>
            </div>
            <div className="navbar-center" data-tauri-drag-region>
                <ul className="menu menu-horizontal px-1 text-base-content">
                    <button 
                        className={`btn btn-xs mx-1 rounded-full ${activeTab === 'launch' ? '' : 'btn-ghost'}`}
                        onClick={() => onNavClick('launch')}
                    >
                        <IoIosRocket className="text-xs" />
                        <span className="text-xs">启动</span>
                    </button>
                    <button 
                        className={`btn btn-xs mx-1 rounded-full ${activeTab === 'download' ? '' : 'btn-ghost'}`}
                        onClick={() => onNavClick('download')}
                    >
                        <FaDownload className="text-xs" />
                        <span className="text-xs">下载</span>
                    </button>
                    <button 
                        className={`btn btn-xs mx-1 rounded-full ${activeTab === 'settings' ? '' : 'btn-ghost'}`}
                        onClick={() => onNavClick('settings')}
                    >
                        <RiSettings6Fill className="text-xs" />
                        <span className="text-xs">设置</span>
                    </button>
                    <button 
                        className={`btn btn-xs mx-1 rounded-full ${activeTab === 'more' ? '' : 'btn-ghost'}`}
                        onClick={() => onNavClick('more')}
                    >
                        <TbAppsFilled className="text-xs" />
                        <span className="text-xs">更多</span>
                    </button>
                </ul>
            </div>
            <div className="navbar-end text-base-content " data-tauri-drag-region>
                <div className='flex items-center'>
                    <ThemeSwitcher/>
                    {/* 添加点击事件处理最小化 */}
                    <button className="btn btn-sm btn-circle btn-ghost" onClick={handleMinimize}>
                        <IoRemove className="text-2xl" />
                    </button>
                    {/* 添加点击事件处理关闭 */}
                    <button className="btn btn-sm btn-ghost btn-circle" onClick={handleClose}>
                        <IoClose className="text-2xl" />
                    </button>
                </div>
            </div>
        </div>
    );
}
