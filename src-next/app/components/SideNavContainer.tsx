"use client";
import React, { useState } from 'react';
import { useUI } from '../context/UIContext';
import LaunchSidebar from './sidebar/LaunchSidebar';
import DownloadSidebar from './sidebar/DownloadSidebar';
import SettingsSidebar from './sidebar/SettingsSidebar';
import MoreSidebar from './sidebar/MoreSidebar';

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
