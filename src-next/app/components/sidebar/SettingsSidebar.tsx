"use client";
import React, { useEffect } from 'react';
import { FiSettings } from 'react-icons/fi';
import { MdOutlineColorLens } from 'react-icons/md';
import { VscCode } from 'react-icons/vsc';
import { useUI } from '../../context/UIContext';

interface SettingsSidebarProps {
  setSideBar: (width: string) => void;
}

export default function SettingsSidebar({ setSideBar }: SettingsSidebarProps) {
  const { uiState, setActiveItem } = useUI();
  const activeItem = uiState.activeItems.settings;

  const menuItems = [
    { id: 'general', name: '常规设置', icon: <FiSettings className="mr-2" /> },
    { id: 'appearance', name: '外观', icon: <MdOutlineColorLens className="mr-2" /> },
    { id: 'devlaunch', name: '启动测试', icon: <VscCode className="mr-2" /> },
  ];
  
  useEffect(() => {
    setSideBar("w-36");
  }, []);
  
  return (
    <div className="h-full w-full">
      <ul className="menu bg-base-200 h-full w-full text-base-content">
        {menuItems.map((item) => (
          <li key={item.id}>
            <a 
              className={`transition-all duration-200 hover:bg-base-300 border-l-4 ${
                activeItem === item.id 
                  ? 'active bg-opacity-20 text-primary font-medium border-primary' 
                  : 'border-transparent'
              }`}
              onClick={() => setActiveItem('settings', item.id)}
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
