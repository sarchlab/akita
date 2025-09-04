import React from 'react';
import { Link } from 'react-router-dom';

interface NavbarProps {
  onChatToggle: () => void;
}

const Navbar: React.FC<NavbarProps> = ({ onChatToggle }) => {
  return (
    <nav className="navbar navbar-dark bg-dark">
      <div className="container-fluid">
        <Link className="navbar-brand" to="/">
          Daisen2
        </Link>
        
        <div className="navbar-nav flex-row">
          <Link className="nav-link me-3" to="/dashboard">
            Dashboard
          </Link>
          <button 
            className="btn btn-outline-light btn-sm"
            onClick={onChatToggle}
          >
            Chat
          </button>
        </div>
      </div>
    </nav>
  );
};

export default Navbar;