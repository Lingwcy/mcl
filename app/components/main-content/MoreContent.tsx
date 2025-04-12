import React from 'react';

interface MoreContentProps {
  activeItem: string | null;
}

export default function MoreContent({ activeItem }: MoreContentProps) {
  if (!activeItem) {
    return (
      <div className="h-full flex items-center justify-center">
        <div className="text-center">
          <h2 className="text-xl font-bold mb-2">更多功能</h2>
          <p>请在侧边栏选择功能</p>
        </div>
      </div>
    );
  }

  const renderContent = () => {
    switch (activeItem) {
      case 'plugins':
        return (
          <div>
          </div>
        );
      case 'logs':
        return (
          <div>
          </div>
        );
      case 'debug':
        return (
          <div>
          </div>
        );
      case 'credits':
        return (
          <div className="flex flex-col items-center justify-center h-full">
            <h2 className="text-2xl font-bold mb-4">鸣谢</h2>
            <p className="mb-4">感谢以下项目和贡献者</p>
            <div className="card bg-base-100 shadow-xl w-full max-w-lg">
              <div className="card-body">
                <h3 className="card-title">开源项目</h3>
                <ul className="list-disc pl-5">
                  <li>React</li>
                  <li>Tauri</li>
                  <li>Next.js</li>
                  <li>Tailwind CSS</li>
                  <li>DaisyUI</li>
                </ul>
              </div>
            </div>
          </div>
        );
      default:
        return null;
    }
  };

  return renderContent();
}
