import React from 'react';

interface StatusBarProps {
  mouseTime: string;
}

const StatusBar: React.FC<StatusBarProps> = ({ mouseTime }) => {
  return (
    <div className="status-bar">
      <div id="mouse-time">
        {mouseTime && `Time: ${mouseTime}`}
      </div>
    </div>
  );
};

export default StatusBar;