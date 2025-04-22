import React, { useState, useEffect } from 'react';
import AccountContext from '../launch-content/AccountsContext';
import InstanceContext from '../launch-content/InstancesContext';  
interface LaunchContentProps {
  activeItem: string | null;
}

export default function LaunchContent({ activeItem }: LaunchContentProps) {
  const [displayedItem, setDisplayedItem] = useState(activeItem);
  const [isVisible, setIsVisible] = useState(true);

  useEffect(() => {
    if (activeItem !== displayedItem) {
      setIsVisible(false); 
      
      const timer = setTimeout(() => {
        setDisplayedItem(activeItem); 
        setIsVisible(true); 

        const endTimer = setTimeout(() => {
        }, 300);
        
        return () => clearTimeout(endTimer);
      }, 300);
      
      return () => clearTimeout(timer);
    }
  }, [activeItem]);

  if (!displayedItem) {
    return (
      <div className="h-full flex items-center justify-center">
        <div className="text-center">
          <h2 className="text-xl font-bold mb-2">启动中心</h2>
          <p>请在侧边栏选择具体功能</p>
        </div>
      </div>
    );
  }

  const renderContent = () => {
    switch (displayedItem) {
      case 'accounts':
        return <AccountContext />;
      case 'instances':
        return <InstanceContext />;
      case 'history':
        return (
          <div>
          </div>
        );
      default:
        return null;
    }
  };

  return (
    <div 
      className={`transition-all duration-300 ease-in-out h-full
                ${isVisible ? 'opacity-100' : 'opacity-0'}`}
    >
      {renderContent()}
    </div>
  );
}
