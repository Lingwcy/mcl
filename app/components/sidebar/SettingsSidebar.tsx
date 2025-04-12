import React, { useEffect } from 'react';
import { FiSettings } from 'react-icons/fi';
import { DiJava } from 'react-icons/di';
import { MdOutlineColorLens } from 'react-icons/md';
import { TbNetwork } from 'react-icons/tb';
import { AiOutlineInfoCircle } from 'react-icons/ai';

interface SettingsSidebarProps {
  onItemSelect: (item: string) => void;
  activeItem: string | null;
  setSideBar: (width: string) => void;
}

export default function SettingsSidebar({ onItemSelect, activeItem, setSideBar }: SettingsSidebarProps) {
  const menuItems = [
    { id: 'general', name: '常规设置', icon: <FiSettings className="mr-2" /> },
    { id: 'java', name: 'Java 设置', icon: <DiJava className="mr-2" /> },
    { id: 'appearance', name: '外观', icon: <MdOutlineColorLens className="mr-2" /> },
    { id: 'network', name: '网络', icon: <TbNetwork className="mr-2" /> },
    { id: 'about', name: '关于', icon: <AiOutlineInfoCircle className="mr-2" /> },
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
              onClick={() => onItemSelect(item.id)}
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
