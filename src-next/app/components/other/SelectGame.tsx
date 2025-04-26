"use client";
import React from 'react';
import { useUI } from '../../context/UIContext';
import { useState } from 'react';
import { open } from '@tauri-apps/plugin-dialog';
import { PathApi } from 'app/rust-api/PathApi';

export interface SelectGameProps {
  isOpen: boolean;
  setIsOpen: (isOpen: boolean) => void;
  onSuccessfulSelect?: () => Promise<void>;
}

export default function SelectGame({ isOpen, setIsOpen, onSuccessfulSelect }: SelectGameProps) {
  const [customName, setCustomName] = useState('');
  const [isCustomNameMode, setIsCustomNameMode] = useState(false);
  const [selectedPath, setSelectedPath] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);

  const handleSelectFolder = async () => {
    try {
      const selected = await open({
        directory: true,
        multiple: false,
        title: '选择 Minecraft 文件夹'
      });

      if (selected && typeof selected === 'string') {
        setSelectedPath(selected);
        setError(null);
      }
    } catch (err) {
      setError(`选择文件夹时出错: ${err}`);
    }
  };

  const handleAddDirectory = async () => {
    if (!selectedPath) {
      setError('请先选择一个文件夹');
      return;
    }

    setIsLoading(true);
    setError(null);

    try {
      let result;
      if (isCustomNameMode && customName) {
        result = await PathApi.addMinecraftPathWithName(selectedPath, customName);
      } else {
        result = await PathApi.addMinecraftPath(selectedPath);
      }

      
      // Clear form
      setSelectedPath('');
      setCustomName('');
      setIsCustomNameMode(false);
      
      // Close modal
      setIsOpen(false);
      
      // Important: Call the refresh callback after successful addition
      if (onSuccessfulSelect) {
        await onSuccessfulSelect();
      }
    } catch (err) {
      setError(`添加文件夹时出错: ${err}`);
    } finally {
      setIsLoading(false);
    }
  };

  if (!isOpen) return null;

  return (
    <div className="modal modal-open">
      <div className="modal-box">
        <h3 className="font-bold text-lg">添加 Minecraft 文件夹</h3>
        
        <div className="form-control w-full mt-4">
          <label className="label">
            <span className="label-text">Minecraft 文件夹路径</span>
          </label>
          <div className="flex gap-2">
            <input 
              type="text" 
              value={selectedPath} 
              readOnly 
              className="input input-bordered flex-1" 
              placeholder="选择 Minecraft 文件夹..." 
            />
            <button 
              className="btn btn-primary" 
              onClick={handleSelectFolder}
              disabled={isLoading}
            >
              浏览...
            </button>
          </div>
        </div>

        <div className="form-control w-full mt-4">
          <label className="label cursor-pointer">
            <span className="label-text">使用自定义显示名称</span>
            <input 
              type="checkbox" 
              className="toggle toggle-primary" 
              checked={isCustomNameMode}
              onChange={() => setIsCustomNameMode(!isCustomNameMode)}
              disabled={isLoading}
            />
          </label>
        </div>

        {isCustomNameMode && (
          <div className="form-control w-full mt-2">
            <input 
              type="text" 
              value={customName} 
              onChange={(e) => setCustomName(e.target.value)} 
              className="input input-bordered w-full" 
              placeholder="输入自定义名称..."
              disabled={isLoading}
            />
          </div>
        )}

        {error && (
          <div className="alert alert-error mt-4">
            <span>{error}</span>
          </div>
        )}

        <div className="modal-action">
          <button 
            className="btn btn-ghost" 
            onClick={() => setIsOpen(false)}
            disabled={isLoading}
          >
            取消
          </button>
          <button 
            className="btn btn-primary" 
            onClick={handleAddDirectory}
            disabled={isLoading || !selectedPath}
          >
            {isLoading ? (
              <>
                <span className="loading loading-spinner loading-sm"></span>
                处理中...
              </>
            ) : '添加'}
          </button>
        </div>
      </div>
    </div>
  );
}