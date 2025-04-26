"use client";
import React from 'react';
import AccountContext from '../launch-content/AccountsContent';
import InstanceContext from '../launch-content/InstancesContent';  
import { useUI } from '../../context/UIContext';

export default function InstanceContent() {
  const { uiState } = useUI();
  const { activeItems } = uiState;
  const activeItem = activeItems.launch;

  if (!activeItem) {
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
    switch (activeItem) {
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
    <div className="h-full">
      {renderContent()}
    </div>
  );
}
