import React from 'react';
import './LoadingSpinner.css';

interface LoadingSpinnerProps {
  size?: 'small' | 'medium' | 'large';
  text?: string;
}

const LoadingSpinner: React.FC<LoadingSpinnerProps> = ({ 
  size = 'medium', 
  text 
}) => {
  return (
    <div className="loading-spinner-container">
      <div className={`loading-spinner spinner-${size}`}>
        <div className="spinner-ring" />
        <div className="spinner-ring" />
        <div className="spinner-ring" />
      </div>
      {text && <span className="spinner-text">{text}</span>}
    </div>
  );
};

export default LoadingSpinner;
