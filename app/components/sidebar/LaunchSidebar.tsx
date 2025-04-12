import React, { useEffect } from 'react';
import { VscDebugStart } from "react-icons/vsc";
import { FiPackage } from "react-icons/fi";
import { RiChatHistoryLine } from "react-icons/ri";

interface MenuItem {
  id: string;
  name: string;
  icon: React.ReactElement;
  children?: MenuItem[];
}
  const menuItems: MenuItem[] = [
    { id: 'accounts', name: '开始', icon: <VscDebugStart  className="mr-2" /> },
    { id: 'instances', name: '实例', icon: <FiPackage className="mr-2" /> },
    { id: 'history', name: '历史', icon: <RiChatHistoryLine  className="mr-2" /> }
  ];

interface LaunchSidebarProps {
  onItemSelect: (id: string) => void;
  activeItem: string | null;
  setSideBar: (width: string) => void;
}

export default function LaunchSidebar({ onItemSelect, activeItem, setSideBar }: LaunchSidebarProps) {
  useEffect(() => {
    setSideBar("w-28");
  }, []); 
  
  const handleItemClick = (id: string) => {
    if (id !== activeItem) {
      onItemSelect(id);
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
