"use client";
import { useState, useEffect } from 'react';
import TopNavBar from './components/TopSiderNavBar';
import LaunchSidebar from './components/sidebar/LaunchSidebar';
import DownloadSidebar from './components/sidebar/DownloadSidebar';
import SettingsSidebar from './components/sidebar/SettingsSidebar';
import LaunchContent from './components/main-content/LaunchContent';
import DownloadContent from './components/main-content/DownloadContent';
import SettingsContent from './components/main-content/SettingsContent';
import MoreContent from './components/main-content/MoreContent';
import MoreSidebar from './components/sidebar/MoreSidebar';
export default function Home() {
  const [activeTab, setActiveTab] = useState<string | null>('launch');
  const [activeSidebarItem, setActiveSidebarItem] = useState<string | null>('accounts');
  const [sidebarWidth, setSidebarWidth] = useState<string>("w-64");
  
  useEffect(() => {
    if (!activeTab) {
      setSidebarWidth("w-0");
    }
  }, [activeTab]);


  const shouldApplyGlassEffect = !(activeTab === 'launch' && activeSidebarItem === 'accounts');

  const handleNavClick = (tab: string) => {
    if (activeTab === tab) {
      setActiveTab(null);
      setActiveSidebarItem(null);
    } else {
      setActiveTab(tab);
      setActiveSidebarItem(null);
    }
  };

  const renderSidebar = () => {
    switch(activeTab) {
      case 'launch':
        return <LaunchSidebar onItemSelect={setActiveSidebarItem} activeItem={activeSidebarItem} setSideBar= {setSidebarWidth} />;
      case 'download':
        return <DownloadSidebar onItemSelect={setActiveSidebarItem} activeItem={activeSidebarItem} setSideBar= {setSidebarWidth}/>;
      case 'settings':
        return <SettingsSidebar onItemSelect={setActiveSidebarItem} activeItem={activeSidebarItem} setSideBar= {setSidebarWidth}/>;
      case 'more':
        return <MoreSidebar onItemSelect={setActiveSidebarItem} activeItem={activeSidebarItem} setSideBar= {setSidebarWidth}/>;
      default:
        return null;
    }
  };

  const renderMainContent = () => {
    if (!activeTab) {
      return (
        <div className="flex items-center justify-center h-full">
          <div className="text-center">
            <h2 className="text-2xl font-bold mb-4">Meow Craft Launcher for Minecraft</h2>
            <p>请在顶部导航栏选择功能</p>
          </div>
        </div>
      );
    }
    
    switch(activeTab) {
      case 'launch':
        return <LaunchContent activeItem={activeSidebarItem} />;
      case 'download':
        return <DownloadContent activeItem={activeSidebarItem} />;
      case 'settings':
        return <SettingsContent activeItem={activeSidebarItem} />;
      case 'more':
        return <MoreContent activeItem={activeSidebarItem} />;
      default:
        return null;
    }
  };

  return (
    <div className="flex flex-col h-screen">
      {/* 顶部导航栏 */}
      <TopNavBar activeTab={activeTab} onNavClick={handleNavClick} />
      
      {/* 主界面显示区域 */}
      <div className="flex flex-1 overflow-hidden">
        {/* 侧边栏 - 带动画效果 */}
        <aside className={`${sidebarWidth} overflow-y-auto transition-all duration-300 ease-in-out`}>
          {renderSidebar()}
        </aside>
        
        {/* 主内容区域 */}
        <main className="flex-1 relative overflow-y-auto bg-primary">
          {/* Wallpaper background */}
          <div 
            className="absolute inset-0 bg-cover bg-center z-0" 
            style={{ backgroundImage: "url('background/bg2.png')" }}
          ></div>
          
          {shouldApplyGlassEffect ? (
            <div className="absolute inset-0 backdrop-blur-md bg-white/30 dark:bg-black/30 z-10"></div>
          ):
          <div className="absolute inset-0 bg-white/30 dark:bg-black/30 z-10"></div>}
          
          {/* Content container */}
          <div className="relative z-20 p-6 h-full">
            {renderMainContent()}
          </div>
        </main>
      </div>
    </div>
  );
}
