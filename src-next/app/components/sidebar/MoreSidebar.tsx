import React, { useEffect } from 'react';
import { FiPackage } from 'react-icons/fi';
import { VscOutput } from 'react-icons/vsc';
import { HiOutlineCode } from 'react-icons/hi';
import { FaRegHandshake } from 'react-icons/fa';

interface MoreSidebarProps {
  onItemSelect: (item: string) => void;
  activeItem: string | null;
  setSideBar: (width: string) => void;
}

export default function MoreSidebar({ onItemSelect, activeItem, setSideBar }: MoreSidebarProps) {
  const menuItems = [
    { id: 'plugins', name: '插件中心', icon: <FiPackage className="mr-2" /> },
    { id: 'logs', name: '日志', icon: <VscOutput className="mr-2" /> },
    { id: 'debug', name: '调试工具', icon: <HiOutlineCode className="mr-2" /> },
    { id: 'credits', name: '鸣谢', icon: <FaRegHandshake className="mr-2" /> }
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
