import React from 'react';
import NormalContext from '../setting-content/NormalContext';
interface SettingsContentProps {
  activeItem: string | null;
}

export default function SettingsContent({ activeItem }: SettingsContentProps) {
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
      case 'about':
        return (
          <div className="flex flex-col items-center justify-center h-full">
            <h2 className="text-2xl font-bold mb-4">关于 Meow Craft Launcher</h2>
            <p className="mb-2">v0.0.1</p>
            <p className="mb-4">精进现代化的 Minecraft 启动器</p>
            <div className="flex gap-2">
              <button className="btn btn-sm">检查更新</button>
              <button className="btn btn-sm">访问官网</button>
            </div>
          </div>
        );
      default:
        return null;
    }
  };

  return renderContent();
}
