"use client";
import React, { createContext, useContext, useState, ReactNode, useCallback } from 'react';

// Define types for our context
export interface UIState {
  activeTab: string | null;
  activeItems: {
    launch: string | null;
    download: string | null;
    settings: string | null;
    more: string | null;
  };
}

interface UIContextType {
  uiState: UIState;
  setActiveTab: (tab: string) => void;
  setActiveItem: (tab: string, item: string | null) => void;
  triggerInstancesRefresh: () => void;
}

// Create the context with default values
const UIContext = createContext<UIContextType | undefined>(undefined);

// Initial UI state
const initialUIState: UIState = {
  activeTab: 'launch',
  activeItems: {
    launch: null,
    download: null,
    settings: null,
    more: null,
  }
};

// Provider component
export function UIProvider({ children }: { children: ReactNode }) {
  const [uiState, setUIState] = useState<UIState>(initialUIState);
  
  // For refresh events (placeholder)
  const triggerInstancesRefresh = useCallback(() => {
    console.log("Instance refresh requested");
  }, []);

  // Function to set active tab
  const setActiveTab = (tab: string) => {
    setUIState(prevState => ({
      ...prevState,
      activeTab: tab
    }));
  };

  // Function to set active item for a specific tab
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

// Custom hook to use the UI context
export function useUI() {
  const context = useContext(UIContext);
  if (context === undefined) {
    throw new Error('useUI must be used within a UIProvider');
  }
  return context;
}
