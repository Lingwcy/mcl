"use client";
import React from 'react';
import NormalContext from '../setting-content/NormalContext';
import DevLaunchContext from '../setting-content/DevLaunchContext';
import { useUI } from '../../context/UIContext';

export default function SettingsContent() {
  const { uiState } = useUI();
  const activeItem = uiState.activeItems.settings;

  if (!activeItem) {
    return (
      <div className="h-full flex items-center justify-center">
        <div className="text-center">
          <h2 className="text-xl font-bold mb-2">设置中心</h2>
          <p>请在侧边栏选择设置类别</p>
        </div>
      </div>
    );
  }

  const renderContent = () => {
    switch (activeItem) {
      case 'general':
        return (
            <NormalContext />
        );
      case 'java':
        return (
          <div>
          </div>
        );
      case 'appearance':
        return (
          <div>
          </div>
        );
      case 'network':
        return (
          <div>
          </div>
        );
      case 'devlaunch':
        return (
          <DevLaunchContext />
        );
      default:
        return <NormalContext />;
    }
  };

  return renderContent();
}
