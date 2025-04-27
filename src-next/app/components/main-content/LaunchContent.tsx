"use client";
import React from 'react';
import AccountContext from '../launch-content/AccountsContent';
import InstanceContext from '../launch-content/InstancesContent';  
import { useUI } from '../../context/UIContext';

export default function LaunchContent() {
  const { uiState } = useUI();
  const { activeItems } = uiState;
  const activeItem = activeItems.launch;


  const renderContent = () => {
    switch (activeItem) {
      case 'accounts':
        return <AccountContext />;
      case 'instances':
        return <InstanceContext />;
      default:
        return <AccountContext />;
    }
  };

  return (
    <div className="h-full">
      {renderContent()}
    </div>
  );
}
