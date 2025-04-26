import { useEffect, useState } from 'react';

interface NotificationProps {
  status: 'success' | 'info' | 'error' | 'warning';
  message: string;
  isVisible: boolean;
  onClose: () => void;
  duration?: number; 
}

export default function Notification({ 
  status, 
  message, 
  isVisible, 
  onClose, 
  duration = 3000 
}: NotificationProps) {
  const [isShowing, setIsShowing] = useState(false);
  

  const alertClass = {
    success: 'alert-success',
    info: 'alert-info',
    error: 'alert-error',
    warning: 'alert-warning'
  }[status];
  

  const alertIcon = {
    success: (
      <svg xmlns="http://www.w3.org/2000/svg" className="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M5 13l4 4L19 7" />
      </svg>
    ),
    info: (
      <svg xmlns="http://www.w3.org/2000/svg" className="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
      </svg>
    ),
    error: (
      <svg xmlns="http://www.w3.org/2000/svg" className="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
      </svg>
    ),
    warning: (
      <svg xmlns="http://www.w3.org/2000/svg" className="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
      </svg>
    )
  }[status];

  useEffect(() => {
    let timer: ReturnType<typeof setTimeout>;
    
    if (isVisible) {
      setIsShowing(true);
      
      timer = setTimeout(() => {
        setIsShowing(false);
        setTimeout(onClose, 300);
      }, duration);
    } else {
      setIsShowing(false);
    }
    
    return () => {
      if (timer) clearTimeout(timer);
    };
  }, [isVisible, duration, onClose]);

  if (!isVisible && !isShowing) return null;

  return (
    <div 
      className={`fixed top-4 right-4 z-50 transition-all duration-300 ease-in-out ${
        isShowing ? 'translate-x-0 opacity-100' : 'translate-x-full opacity-0'
      }`}
      style={{ maxWidth: '90vw', minWidth: '300px' }}
    >
      <div className={`alert ${alertClass} shadow-lg`}>
        <div className="flex items-center">
          {alertIcon}
          <span>{message}</span>
        </div>
        <div className="flex-none">
          <button onClick={() => {
            setIsShowing(false);
            setTimeout(onClose, 300);
          }} className="btn btn-sm btn-ghost">
            <svg xmlns="http://www.w3.org/2000/svg" className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
      </div>
    </div>
  );
}
