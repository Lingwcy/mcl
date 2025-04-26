"use client";
import React, { useState } from 'react';
import { useUI } from '../../context/UIContext';

interface CollapseState {
  pureMinecraft: boolean;
  latestVersions: boolean;
}

export default function DownloadContent() {
  const { uiState } = useUI();
  const activeItem = uiState.activeItems.download;
  
  const [collapseState, setCollapseState] = useState<CollapseState>({
    pureMinecraft: true,
    latestVersions: true
  });

  const toggleCollapse = (key: keyof CollapseState) => {
    setCollapseState(prev => ({
      ...prev,
      [key]: !prev[key]
    }));
  };

  if (!activeItem) {
    return (
      <div className="h-full flex items-center justify-center">
        <div className="text-center">
          <h2 className="text-xl font-bold mb-2">Miao Craft Launcher</h2>
          <p></p>
        </div>
      </div>
    );
  }

  const renderContent = () => {
    switch (activeItem) {
      case 'minecraft':
        return (
          <div className="h-full w-full flex flex-col items-center">
            <div className={`mb-2 collapse ${collapseState.pureMinecraft ? 'collapse-open' : 'collapse-close'} collapse-arrow bg-base-200 border-base-300 border`}>
              <div 
                className="collapse-title min-h-10 py-2 text-sm font-semibold text-base-content cursor-pointer" 
                onClick={() => toggleCollapse('pureMinecraft')}
              >
                纯净的Minecraft版本
              </div>
              <div className="collapse-content py-1 text-xs text-base-content">
                纯净版为Mojang官方的发行版或预览版, 这通常是没有任何模组和修改的Minecraft版本.
              </div>
            </div>

            <div className={`mb-2 collapse ${collapseState.latestVersions ? 'collapse-open' : 'collapse-close'} collapse-arrow bg-base-200 border-base-300 border`}>
              <div 
                className="collapse-title min-h-10 py-2 text-sm font-semibold text-base-content cursor-pointer" 
                onClick={() => toggleCollapse('latestVersions')}
              >
                最新版本
              </div>
              <div className="collapse-content py-1 text-xs text-base-content">
                <ul className="list bg-base-100 rounded-box shadow-sm">
                  <li className="list-row py-1.5 items-center">
                    <div className="flex items-center justify-center">
                      <img className="size-6 rounded-box" src="blocks/Grass.png" />
                    </div>
                    <div className="ml-2 flex flex-col justify-center">
                      <div className="text-sm leading-tight">1.21.5</div>
                      <div className="text-xs opacity-60 leading-tight">最新正式版</div>
                    </div>
                  </li>

                  <li className="list-row py-1.5 items-center">
                    <div className="flex items-center justify-center">
                      <img className="size-6 rounded-box" src="blocks/CommandBlock.png" />
                    </div>
                    <div className="ml-2 flex flex-col justify-center">
                      <div className="text-sm leading-tight">25w15a</div>
                      <div className="text-xs uppercase opacity-60 leading-tight">最新预览版</div>
                    </div>
                  </li>
                </ul>
              </div>
            </div>
          </div>
        );
      case 'mods':
        return (
          <div>
          </div>
        );
      case 'resourcepacks':
        return (
          <div>
          </div>
        );
      case 'worlds':
        return (
          <div>
          </div>
        );
      default:
        return null;
    }
  };

  return renderContent();
}
