"use client";
import React from 'react';
import TopNavBar from './components/TopSiderNavBar';
import MainContainer from './components/MainContainer';
import SideNavContainer from './components/SideNavContainer';

export default function Home() {
  return (
    <main 
      className="flex flex-col h-screen bg-base-300"
    >
      <TopNavBar />
      <div className="flex flex-1 overflow-hidden">
        <SideNavContainer />
        <MainContainer />
        
      </div>
    </main>
  );
}
