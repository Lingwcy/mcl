"use client";
import React, { useEffect } from 'react';
import { FaCubes } from 'react-icons/fa';
import { HiCube } from "react-icons/hi";
import { useUI } from '../../context/UIContext';

interface DownloadSidebarProps {
  setSideBar: (width: string) => void;
}

interface MenuItem {
  id: string;
  name: string;
  icon?: React.ReactNode; 
  children?: MenuItem[];
}

export default function DownloadSidebar({ setSideBar }: DownloadSidebarProps) {
  const { uiState, setActiveItem } = useUI();
  const activeItem = uiState.activeItems.download;
  
  const menuItems: MenuItem[] = [
    { id: 'minecraft', name: 'Minecraft', icon: <HiCube className="mr-3" /> },
    { 
      id: 'community-resources', 
      name: '社区资源',
      icon: <FaCubes className="mr-1" />,
      children: [
        { id: 'mods', name: 'Mods' },
        { id: 'entity', name: '整合包' }
      ] 
    },
  ];

  useEffect(() => {
    setSideBar("w-36");
  }, []);
  
  return (
    <div className="h-full w-full">
      <ul className="menu bg-base-200 h-full w-full text-base-content">
        {menuItems.map((item) => (
          <li key={item.id}>
            {item.children ? (
              <>
                <span className="flex items-center font-medium">
                  {item.icon} 
                  {item.name}
                </span>
                <ul className="menu-dropdown menu-dropdown-show ml-4"> 
                  {item.children.map(child => (
                    <li key={child.id}>
                      <a 
                        className={`transition-all duration-200 hover:bg-base-300 border-l-4 ${
                          activeItem === child.id 
                            ? 'active bg-opacity-20 text-primary font-medium border-primary' 
                            : 'border-transparent'
                        }`}
                        onClick={() => setActiveItem('download', child.id)}
                      >
                        {child.name}
                      </a>
                    </li>
                  ))}
                </ul>
              </>
            ) : (
              <a 
                className={`transition-all duration-200 hover:bg-base-300 border-l-4 ${
                  activeItem === item.id 
                    ? 'active bg-opacity-20 text-primary font-medium border-primary' 
                    : 'border-transparent'
                }`}
                onClick={() => setActiveItem('download', item.id)}
              >
                <span className="flex items-center">
                  {item.icon} 
                  {item.name}
                </span>
              </a>
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}
