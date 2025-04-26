"use client";
import { createContext, useState, useContext, ReactNode, useEffect } from 'react';
import Notification from '../components/other/Notification';

type NotificationStatus = 'success' | 'info' | 'error' | 'warning';

interface NotificationContextType {
  showNotification: (status: NotificationStatus, message: string, duration?: number) => void;
  hideNotification: () => void;
  isReady: boolean;
}

// Create context with default values
const NotificationContext = createContext<NotificationContextType>({
  showNotification: () => console.warn("NotificationProvider not found"),
  hideNotification: () => {},
  isReady: false
});

export const useNotification = () => {
  const context = useContext(NotificationContext);
  
  // Instead of throwing an error, return the context with defaults
  return context;
};

export function NotificationProvider({ children }: { children: ReactNode }) {
  const [isReady, setIsReady] = useState(false);
  const [notification, setNotification] = useState({
    visible: false,
    status: 'success' as NotificationStatus,
    message: '',
    duration: 3000
  });

  useEffect(() => {
    // Mark the provider as ready after mounting
    setIsReady(true);
    return () => setIsReady(false);
  }, []);

  const showNotification = (status: NotificationStatus, message: string, duration = 5000) => {
    setNotification({
      visible: true,
      status,
      message,
      duration
    });
  };

  const hideNotification = () => {
    setNotification(prev => ({ ...prev, visible: false }));
  };

  const contextValue = {
    showNotification,
    hideNotification,
    isReady
  };

  return (
    <NotificationContext.Provider value={contextValue}>
      {children}
      <Notification
        status={notification.status}
        message={notification.message}
        isVisible={notification.visible}
        onClose={hideNotification}
        duration={notification.duration}
      />
    </NotificationContext.Provider>
  );
}
