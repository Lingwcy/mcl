"use client";
import React, { createContext, useContext, useState, ReactNode, useCallback } from 'react';

export interface UIState {
  activeTab: string | null;
  activeItems: {
    launch: string | null;
    download: string | null;
    settings: string | null;
    more: string | null;
    instanceDetail: string | null;
  };
}

interface UIContextType {
  uiState: UIState;
  setActiveTab: (tab: string) => void;
  setActiveItem: (tab: string, item: string | null) => void;
  triggerInstancesRefresh: () => void;
}

const UIContext = createContext<UIContextType | undefined>(undefined);

const initialUIState: UIState = {
  activeTab: 'launch',
  activeItems: {
    launch: null,
    download: null,
    settings: null,
    more: null,
    instanceDetail: null
  }
};

export function UIProvider({ children }: { children: ReactNode }) {
  const [uiState, setUIState] = useState<UIState>(initialUIState);

  const triggerInstancesRefresh = useCallback(() => {
    console.log("Instance refresh requested");
  }, []);

  const setActiveTab = (tab: string) => {
    setUIState(prevState => ({
      ...prevState,
      activeTab: tab
    }));
  };

  const setActiveItem = (tab: string, item: string | null) => {
    setUIState(prevState => ({
      ...prevState,
      activeItems: {
        ...prevState.activeItems,
        [tab]: item
      }
    }));
  };

  return (
    <UIContext.Provider value={{ 
      uiState, 
      setActiveTab, 
      setActiveItem,
      triggerInstancesRefresh
    }}>
      {children}
    </UIContext.Provider>
  );
}

export function useUI() {
  const context = useContext(UIContext);
  if (context === undefined) {
    throw new Error('useUI must be used within a UIProvider');
  }
  return context;
}
