"use client";
import React, { useEffect } from 'react';

import { FaRegHandshake } from 'react-icons/fa';
import { useUI } from '../../context/UIContext';
import { AiOutlineInfoCircle } from 'react-icons/ai';
interface MoreSidebarProps {
  setSideBar: (width: string) => void;
}

export default function MoreSidebar({ setSideBar }: MoreSidebarProps) {
  const { uiState, setActiveItem } = useUI();
  const activeItem = uiState.activeItems.more;

  const menuItems = [
    { id: 'credits', name: '鸣谢', icon: <FaRegHandshake className="mr-2" /> },
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
              onClick={() => setActiveItem('more', item.id)}
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
