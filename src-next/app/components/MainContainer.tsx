"use client";
import React from 'react';
import { useUI } from '../context/UIContext';
import LaunchContent from './main-content/LaunchContent';
import DownloadContent from './main-content/DownloadContent';
import SettingsContent from './main-content/SettingsContent';
import MoreContent from './main-content/MoreContent';

export default function MainContainer() {
    const { uiState } = useUI();
    const { activeTab } = uiState;

    const renderContent = () => {
        switch (activeTab) {
            case 'launch':
                return <LaunchContent />;
            case 'download':
                return <DownloadContent />;
            case 'settings':
                return <SettingsContent />;
            case 'more':
                return <MoreContent />;
            default:
                return (
                    <div className="h-full flex items-center justify-center">
                        <div className="text-center">
                            <h2 className="text-xl font-bold mb-2">Miao Craft Launcher</h2>
                            <p>Welcome to MCL</p>
                        </div>
                    </div>
                );
        }
    };


    const shouldApplyBlur = activeTab !== 'launch'
    
    return (
        <div className="flex-1 overflow-hidden relative">
            <div 
                className="absolute inset-0 z-0"
                style={{
                    backgroundImage: `url('/background/bg1.png')`,
                    backgroundSize: 'cover',
                    backgroundPosition: 'center',
                    opacity: 1, 
                }}
            ></div>
        
            <div className={`relative z-20 h-full ${shouldApplyBlur ? 'backdrop-blur-xs' : ''}`}>
                {renderContent()}
            </div>
        </div>
    );
}
