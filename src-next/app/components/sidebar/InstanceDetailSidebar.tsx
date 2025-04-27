"use client";
import React, { useEffect } from 'react';
import { VscDebugStart } from "react-icons/vsc";
import { FiPackage } from "react-icons/fi";
import { useUI } from '../../context/UIContext';

interface MenuItem {
  id: string;
  name: string;
  icon: React.ReactElement;
  children?: MenuItem[];
}

const menuItems: MenuItem[] = [
  { id: 'instanceConifg', name: '实例信息', icon: <VscDebugStart className="mr-2" /> },
  { id: 'modsConfig', name: 'mods配置', icon: <FiPackage className="mr-2" /> },
];

interface LaunchSidebarProps {
  setSideBar: (width: string) => void;
}

export default function InstanceDetailSidebar({ setSideBar }: LaunchSidebarProps) {
  const { uiState, setActiveItem } = useUI();
  const activeItem = uiState.activeItems.instanceDetail;
  
  useEffect(() => {
    setSideBar("w-40");
  }, []); 
  
  const handleItemClick = (id: string) => {
    if (id !== activeItem) {
      setActiveItem('instanceDetail', id);
    }
  };
  
  return (
    <div className="h-full w-full">
      <ul className="menu bg-base-200 h-full w-full text-base-content">
        {menuItems.map((item) => (
          <li key={item.id}>
            <a 
              className={`transition-all duration-300 hover:bg-base-300 border-l-4 ${
                activeItem === item.id 
                  ? 'active bg-opacity-20 text-primary font-medium border-primary' 
                  : 'border-transparent'
              }`}
              onClick={() => handleItemClick(item.id)}
            >
              <span className="flex items-center">
                {item.icon} 
                {item.name}
              </span>
            </a>
          </li>
        ))}
      </ul>
    </div>
  );
}
