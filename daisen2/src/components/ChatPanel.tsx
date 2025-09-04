import React, { useState, useRef, useEffect } from 'react';
import { ChatMessage } from '../types';

interface ChatPanelProps {
  isOpen: boolean;
  onClose: () => void;
}

const ChatPanel: React.FC<ChatPanelProps> = ({ isOpen, onClose }) => {
  const [messages, setMessages] = useState<ChatMessage[]>([
    { role: 'system', content: 'You are Daisen Bot.' },
    { role: 'assistant', content: 'Hello! What can I help you with today?' }
  ]);
  const [inputValue, setInputValue] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const messagesEndRef = useRef<HTMLDivElement>(null);

  const scrollToBottom = () => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  };

  useEffect(() => {
    scrollToBottom();
  }, [messages]);

  const sendMessage = async () => {
    if (!inputValue.trim() || isLoading) return;

    const userMessage: ChatMessage = { role: 'user', content: inputValue };
    setMessages(prev => [...prev, userMessage]);
    setInputValue('');
    setIsLoading(true);

    try {
      // Simulate API call - replace with actual chat API
      setTimeout(() => {
        const botResponse: ChatMessage = {
          role: 'assistant',
          content: `I received your message: "${userMessage.content}". This is a demo response from Daisen Bot.`
        };
        setMessages(prev => [...prev, botResponse]);
        setIsLoading(false);
      }, 1000);
    } catch (error) {
      console.error('Failed to send message:', error);
      setIsLoading(false);
    }
  };

  const handleKeyPress = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      sendMessage();
    }
  };

  const clearChat = () => {
    setMessages([
      { role: 'system', content: 'You are Daisen Bot.' },
      { role: 'assistant', content: 'Hello! What can I help you with today?' }
    ]);
  };

  return (
    <div className={`chat-panel d-flex flex-column ${isOpen ? 'open' : ''}`}>
      <div className="chat-header">
        <h5 className="mb-0">Daisen Bot</h5>
        <div>
          <button className="btn btn-sm btn-outline-secondary me-2" onClick={clearChat}>
            Clear
          </button>
          <button className="btn-close" onClick={onClose}></button>
        </div>
      </div>

      <div className="chat-messages flex-grow-1">
        {messages.slice(1).map((message, index) => (
          <div key={index} className={`mb-3 ${message.role === 'user' ? 'text-end' : ''}`}>
            <div 
              className={`d-inline-block p-2 rounded ${
                message.role === 'user' 
                  ? 'bg-primary text-white' 
                  : 'bg-light'
              }`}
              style={{ maxWidth: '80%' }}
            >
              <strong>{message.role === 'user' ? 'You' : 'Daisen Bot'}:</strong> {message.content}
            </div>
          </div>
        ))}
        {isLoading && (
          <div className="mb-3">
            <div className="d-inline-block p-2 rounded bg-light" style={{ maxWidth: '80%' }}>
              <strong>Daisen Bot:</strong> Thinking...
            </div>
          </div>
        )}
        <div ref={messagesEndRef} />
      </div>

      <div className="chat-input-container">
        <div className="input-group">
          <textarea
            className="form-control"
            rows={3}
            value={inputValue}
            onChange={(e) => setInputValue(e.target.value)}
            onKeyPress={handleKeyPress}
            placeholder="Type your message..."
            disabled={isLoading}
          />
          <button 
            className="btn btn-primary" 
            onClick={sendMessage}
            disabled={isLoading || !inputValue.trim()}
          >
            Send
          </button>
        </div>
      </div>
    </div>
  );
};

export default ChatPanel;