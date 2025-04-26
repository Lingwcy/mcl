"use client";
import React from 'react';
import { useUI } from '../../context/UIContext';

export default function MoreContent() {
  const { uiState } = useUI();
  const activeItem = uiState.activeItems.more;

  if (!activeItem) {
    return (
      <div className="h-full flex items-center justify-center">
        <div className="text-center">
          <h2 className="text-xl font-bold mb-2">更多功能</h2>
          <p>请在侧边栏选择功能</p>
        </div>
      </div>
    );
  }


  const renderContent = () => {
    switch (activeItem) {
      case 'plugins':
        return (
          <div>
          </div>
        );
      case 'logs':
        return (
          <div>
          </div>
        );
      case 'about':
        return (
          <div className="flex flex-col items-center justify-center h-full">
            <h2 className="text-2xl font-bold mb-4">Meow Craft Launcher</h2>
            <p className="mb-2">v0.0.2</p>
            <p className="mb-4">精进现代化的 Minecraft 启动器</p>
            <div className="flex gap-2">
              <button className="btn bg-black text-white border-black">
                <svg aria-label="GitHub logo" width="16" height="16" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="white" d="M12,2A10,10 0 0,0 2,12C2,16.42 4.87,20.17 8.84,21.5C9.34,21.58 9.5,21.27 9.5,21C9.5,20.77 9.5,20.14 9.5,19.31C6.73,19.91 6.14,17.97 6.14,17.97C5.68,16.81 5.03,16.5 5.03,16.5C4.12,15.88 5.1,15.9 5.1,15.9C6.1,15.97 6.63,16.93 6.63,16.93C7.5,18.45 8.97,18 9.54,17.76C9.63,17.11 9.89,16.67 10.17,16.42C7.95,16.17 5.62,15.31 5.62,11.5C5.62,10.39 6,9.5 6.65,8.79C6.55,8.54 6.2,7.5 6.75,6.15C6.75,6.15 7.59,5.88 9.5,7.17C10.29,6.95 11.15,6.84 12,6.84C12.85,6.84 13.71,6.95 14.5,7.17C16.41,5.88 17.25,6.15 17.25,6.15C17.8,7.5 17.45,8.54 17.35,8.79C18,9.5 18.38,10.39 18.38,11.5C18.38,15.32 16.04,16.16 13.81,16.41C14.17,16.72 14.5,17.33 14.5,18.26C14.5,19.6 14.5,20.68 14.5,21C14.5,21.27 14.66,21.59 15.17,21.5C19.14,20.16 22,16.42 22,12A10,10 0 0,0 12,2Z"></path></svg>
                Visit In Github Repository
              </button>
            </div>
          </div>
        );
      case 'credits':
        return (
          <div className="flex flex-col items-center justify-center h-full p-2">
            <div className="text-center mb-4">
              <p className="text-xl text-base-content ">感谢这些优秀的项目</p>
              <div className="divider max-w-xs mx-auto my-1"></div>
            </div>

            <div className="grid grid-cols-2 gap-3 w-full max-w-2xl text-base-content">
              {/* 核心技术 */}
              <div className="card bg-base-200 shadow-md hover:shadow-lg transition-all ">
                <div className="card-body p-3">
                  <div className="flex justify-between items-center">
                    <h3 className="card-title text-sm">核心技术</h3>
                    <div className="badge badge-sm badge-primary">Core</div>
                  </div>
                  <div className="divider my-0"></div>
                  <div className="grid grid-cols-1 gap-1 text-sm">
                    <div className="flex items-center gap-1">
                      <div className="w-5 h-5 rounded-full bg-blue-500 flex items-center justify-center text-white text-xs font-bold">R</div>
                      <span>React</span>
                    </div>
                    <div className="flex items-center gap-1">
                      <div className="w-5 h-5 rounded-full bg-black flex items-center justify-center text-white text-xs font-bold">N</div>
                      <span>Next.js</span>
                    </div>
                    <div className="flex items-center gap-1">
                      <div className="w-5 h-5 rounded-full bg-orange-500 flex items-center justify-center text-white text-xs font-bold">T</div>
                      <span>Tauri</span>
                    </div>
                  </div>
                </div>
              </div>

              {/* UI库 */}
              <div className="card bg-base-200 shadow-md hover:shadow-lg transition-all">
                <div className="card-body p-3">
                  <div className="flex justify-between items-center">
                    <h3 className="card-title text-sm">UI 库</h3>
                    <div className="badge badge-sm badge-accent">Design</div>
                  </div>
                  <div className="divider my-0"></div>
                  <div className="grid grid-cols-1 gap-1 text-sm">
                    <div className="flex items-center gap-1">
                      <div className="w-5 h-5 rounded-full bg-sky-500 flex items-center justify-center text-white text-xs font-bold">T</div>
                      <span>Tailwind CSS</span>
                    </div>
                    <div className="flex items-center gap-1">
                      <div className="w-5 h-5 rounded-full bg-purple-500 flex items-center justify-center text-white text-xs font-bold">D</div>
                      <span>DaisyUI</span>
                    </div>
                  </div>
                </div>
              </div>

            </div>

            <div className="mt-3 text-center text-xs opacity-70">
              <p>Version: Dev 0.0.2 | 2023 © MIAOTOWN All Rights Reserved</p>
            </div>
          </div>
        );
      default:
        return null;
    }
  };

  return renderContent();
}
