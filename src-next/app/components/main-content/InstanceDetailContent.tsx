"use client";
import React from 'react';
import ModsManagementContent from '../instanceDetail/ModsManager';
import { useUI } from '../../context/UIContext';

export default function InstanceContent() {
  const { uiState } = useUI();
  const { activeItems } = uiState;
  const activeItem = activeItems.instanceDetail;

  if (!activeItem) {
    return (
      <div className="h-full flex items-center justify-center">
      </div>
    );
  }

  const renderContent = () => {
    switch (activeItem) {
      case 'instanceConifg':
        return <>
          <div className="badge badge-soft badge-primary">Primary</div>
          <div className="badge badge-soft badge-secondary">Secondary</div>
          <div className="badge badge-soft badge-accent">Accent</div>
          <div className="badge badge-soft badge-info">Info</div>
          <div className="badge badge-soft badge-success">Success</div>
          <div className="badge badge-soft badge-warning">Warning</div>
          <div className="badge badge-soft badge-error">Error</div>
        </>;
      case 'modsConfig':
        return <ModsManagementContent />
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
