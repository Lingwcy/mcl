"use client";
import { useState } from 'react';
import { IoCloudOffline } from "react-icons/io5";
import { MdSwitchLeft, MdSwitchRight } from "react-icons/md";
import { IoChevronDown, IoGameController } from "react-icons/io5";

export default function AccountContext() {
  const [userId, setUserId] = useState('Ou_Takahiros');
  const [microsoftName, setMicrosoftName] = useState('');
  const [isOffline, setIsOffline] = useState(true);
  const [isEditing, setIsEditing] = useState(false);
  const [selectedInstance, setSelectedInstance] = useState('Vanilla 1.20.4');
  const [isModalOpen, setIsModalOpen] = useState(false);

  const instances = [
    { name: 'Vanilla 1.20.4', type: 'vanilla', version: '1.20.4', clientType: 'Vanilla' },
    { name: 'Forge 1.19.2', type: 'forge', version: '1.19.2', clientType: 'Forge' },
    { name: 'Fabric 1.20.1', type: 'fabric', version: '1.20.1', clientType: 'Fabric' },
    { name: 'Miao Town VI', type: 'modpack', version: '1.18.2', clientType: 'CurseForge' },
  ];

  const handleUserIdChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setUserId(e.target.value);
  };

  const toggleEditMode = () => {
    if (isOffline) {
      setIsEditing(!isEditing);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      setIsEditing(false);
    }
  };

  return (
    <div className="flex flex-col h-full items-end justify-end">
      <div className="w-50 flex flex-col">
        <div className="flex flex-col w-full h-30 bg-base-200 text-base-content p-2 justify-center items-center opacity-95 shadow-md">
          <div className="w-14 h-14 mb-1 rounded-sm overflow-hidden flex-shrink-0 mt-3">
            <div className="w-full h-full bg-gradient-to-br from-blue-500 to-purple-500"></div>
          </div>
          <div className="text-base-content text-xs w-full mt-1">
            {isEditing ? (
              <input
                type="text"
                value={userId}
                onChange={handleUserIdChange}
                onBlur={() => setIsEditing(false)}
                onKeyDown={handleKeyDown}
                className="text-base-content text-center w-full "
                autoFocus
              />
            ) : (
              <div>
                {isOffline ? (
                  <div
                    className={`text-center ${isOffline ? "cursor-pointer hover:underline" : ""}`}
                    onClick={toggleEditMode}
                  >
                    {userId}
                  </div>
                ) : (
                  <div className="flex flex-col items-center">
                    {microsoftName ? (
                      <span className="text-xs">{microsoftName}</span>
                    ):(<
                      div className="flex items-center justify-center w-full">
                        <button className="btn btn-bl btn-xs w-20 h-5 min-h-0 px-2" onClick={() => setIsOffline(true)}>登录</button>
                      </div>
                    )}
                  </div>
                )}
              </div>
            )}
          </div>
        </div>
        <div className="flex flex-col w-full h-24 relative opacity-95 shadow-md">
          <div className="flex flex-row w-full h-1/4">
            <div
              className={`w-full h-full flex items-center justify-center cursor-pointer`}
              onClick={() => setIsOffline(false)}
            >
              <label className="rounded-none bg-base-300 text-base-content w-full h-full flex justify-between items-center px-3">
                <div className="flex items-center">
                  {!isOffline ? (
                    <svg aria-label="Microsoft logo" width="16" height="16" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512" className="mr-1">
                      <path d="M96 96H247V247H96" fill="#f24f23"></path>
                      <path d="M265 96V247H416V96" fill="#7eba03"></path>
                      <path d="M96 265H247V416H96" fill="#3ca4ef"></path>
                      <path d="M265 265H416V416H265" fill="#f9ba00"></path>
                    </svg>
                  ) :
                    <IoCloudOffline className="text-xs mr-1" />}
                  <span className="text-xs">
                    {isOffline ? "离线" : "正版"}
                  </span>
                </div>

                <div className="swap swap-rotate">
                  <input
                    type="checkbox"
                    checked={isOffline}
                    onChange={() => setIsOffline(!isOffline)}
                    onClick={(e) => {
                      e.stopPropagation();
                      setIsOffline(!isOffline);
                    }}
                    className="z-10 cursor-pointer"
                  />
                  <MdSwitchRight className="swap-off" />
                  <MdSwitchLeft className="swap-on" />
                </div>
              </label>
            </div>
          </div>
          <div className="w-full h-3/4 flex">
            <button
              className="flex-1 bg-base-100 text-base-content flex items-center justify-center font-bold cursor-pointer hover:bg-base-200 transition-colors duration-200"
              onClick={() => console.log("Launching game with instance:", selectedInstance)}
            >
              <div className="flex items-center gap-2 px-2">
                <IoGameController className="text-lg" />
                <span className="truncate">{selectedInstance}</span>
              </div>
            </button>

            <button
              className="w-10 text-base-content bg-base-100 border-l border-base-300 flex items-center justify-center hover:bg-base-200 transition-colors duration-200"
              onClick={() => setIsModalOpen(true)}
            >
              <IoChevronDown className="transition-transform duration-200" />
            </button>
          </div>

          {isModalOpen && (
            <div className="fixed inset-0 backdrop-blur-sm flex items-center justify-center z-50">
              <div className="bg-base-100 text-base-content p-6 rounded-lg shadow-lg max-w-md w-full">
                <h3 className="font-bold text-lg">选择一个实例包</h3>
                <div className="py-4 max-h-60 overflow-y-auto">
                  {instances.map((instance, index) => (
                    <div
                      key={index}
                      className="px-4 py-2 hover:bg-base-300 cursor-pointer flex items-center gap-2 mb-1 rounded"
                      onClick={() => {
                        setSelectedInstance(instance.name);
                        setIsModalOpen(false);
                      }}
                    >
                      <span className={`w-2 h-2 rounded-full ${instance.type === 'vanilla' ? 'bg-green-500' :
                        instance.type === 'forge' ? 'bg-amber-500' :
                          instance.type === 'fabric' ? 'bg-blue-500' :
                            'bg-purple-500'
                        }`}></span>
                      <div className="flex flex-col">
                        <span>{instance.name}</span>
                        <span className="text-xs text-gray-400">{instance.version} • {instance.clientType}</span>
                      </div>
                    </div>
                  ))}
                </div>
                <div className="flex justify-end mt-4">
                  <button
                    className="btn"
                    onClick={() => setIsModalOpen(false)}
                  >
                    Close
                  </button>
                </div>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}