"use client";

import { useState, useEffect } from "react";
import { MdLightMode } from "react-icons/md";
// Define available themes
const THEMES = [
  { name: "light", label: "珍珠白" },
  { name: "dark", label: "暗黑" },
  { name: "retro", label: "古典" },
  { name: "cyberpunk", label: "Cyberpunk" },
  { name: "valentine", label: "Valentine" },
  { name: "aqua", label: "Aqua" }
];

export default function ThemeSwitcher() {
  const [theme, setTheme] = useState<string>("light");

  // Effect to initialize theme from localStorage or system preference
  useEffect(() => {
    // Check localStorage
    const savedTheme = localStorage.getItem("theme");
    
    if (savedTheme && THEMES.some(t => t.name === savedTheme)) {
      setTheme(savedTheme);
      document.documentElement.setAttribute("data-theme", savedTheme);
    } else {
      // Check system preference for light/dark, default to light otherwise
      const prefersDark = window.matchMedia("(prefers-color-scheme: dark)").matches;
      const defaultTheme = prefersDark ? "dark" : "light";
      setTheme(defaultTheme);
      document.documentElement.setAttribute("data-theme", defaultTheme);
      localStorage.setItem("theme", defaultTheme);
    }
  }, []);

  // Theme change handler
  const changeTheme = (newTheme: string) => {
    setTheme(newTheme);
    document.documentElement.setAttribute("data-theme", newTheme);
    localStorage.setItem("theme", newTheme);
  };

  return (
    <div className="dropdown dropdown-end">
      <div 
        tabIndex={0} 
        role="button" 
        className="btn btn-sm btn-circle btn-ghost relative"
        style={{ overflow: 'visible' }}
      >
        <MdLightMode className="text-xl" />
        <div 
          className="absolute rounded-full border-2 border-base-100" 
          style={{
            ...getThemeColorStyle(theme),
            width: '10px',
            height: '10px',
            bottom: '13%',
            right: '10%',
            transform: 'translate(20%, 20%)',
          }}
        ></div>
      </div>
      <ul tabIndex={0} className="dropdown-content z-[1] menu p-2 shadow bg-base-100 rounded-box w-15">
        {THEMES.map((t) => (
          <li key={t.name} onClick={() => changeTheme(t.name)}>
            <a className={theme === t.name ? "active" : ""}>
              <div className="flex items-center">
                <div className={`w-4 h-4 rounded-full mr-2 theme-color-${t.name}`} style={getThemeColorStyle(t.name)}></div>
              </div>
            </a>
          </li>
        ))}
      </ul>
    </div>
  );
}

// Helper function to provide theme color indicators
function getThemeColorStyle(themeName: string) {
  const themeColors: Record<string, string> = {
    light: "#f8fafc",
    dark: "#1e293b",
    retro: "#d1c7a3",
    cyberpunk: "#ff7598",
    valentine: "#e96d7b",
    aqua: "#5adae8"
  };
  
  return {
    backgroundColor: themeColors[themeName] || "#cbd5e1",
    border: themeName === "light" ? "1px solid #cbd5e1" : "none"
  };
}