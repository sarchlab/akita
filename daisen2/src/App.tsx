import { useState } from 'react';
import { Routes, Route } from 'react-router-dom';
import Navbar from './components/Navbar';
import Dashboard from './pages/Dashboard';
import TaskView from './pages/TaskView';
import ComponentView from './pages/ComponentView';
import ChatPanel from './components/ChatPanel';
import StatusBar from './components/StatusBar';

function App() {
  const [chatOpen, setChatOpen] = useState(false);
  const [mouseTime, setMouseTime] = useState<string>('');

  const toggleChat = () => {
    setChatOpen(!chatOpen);
  };

  return (
    <div className="App">
      <Navbar onChatToggle={toggleChat} />
      
      <div className="main-body">
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/dashboard" element={<Dashboard />} />
          <Route path="/task" element={<TaskView onMouseTimeUpdate={setMouseTime} />} />
          <Route path="/component" element={<ComponentView onMouseTimeUpdate={setMouseTime} />} />
        </Routes>
      </div>

      <ChatPanel isOpen={chatOpen} onClose={() => setChatOpen(false)} />
      <StatusBar mouseTime={mouseTime} />
    </div>
  );
}

export default App;