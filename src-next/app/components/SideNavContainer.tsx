"use client";
import React, { useState } from 'react';
import { useUI } from '../context/UIContext';
import LaunchSidebar from './sidebar/LaunchSidebar';
import DownloadSidebar from './sidebar/DownloadSidebar';
import SettingsSidebar from './sidebar/SettingsSidebar';
import MoreSidebar from './sidebar/MoreSidebar';
import InstanceDetailSidebar from './sidebar/InstanceDetailSidebar';

/*
  侧边栏控制
  方式：数据驱动
  逻辑：当UIContext中的uiState发生变化时，此组件渲染不同的sidebar

*/
export default function SideNavContainer() {
  const { uiState } = useUI();
  const { activeTab } = uiState;
  const [sidebarWidth, setSidebarWidth] = useState("w-28");

  const renderSidebar = () => {
    switch (activeTab) {
      case 'launch':
        return <LaunchSidebar setSideBar={setSidebarWidth} />;
      case 'download':
        return <DownloadSidebar setSideBar={setSidebarWidth} />;
      case 'settings':
        return <SettingsSidebar setSideBar={setSidebarWidth} />;
      case 'more':
        return <MoreSidebar setSideBar={setSidebarWidth} />;
      case 'instanceDetail':
        return <InstanceDetailSidebar setSideBar={setSidebarWidth}/>
      default: 
        return null;
    }
  };

  return (
    <div className={`h-full ${sidebarWidth} transition-all duration-300 bg-base-200 border-r border-base-300`}>
      {renderSidebar()}
    </div>
  );
}
