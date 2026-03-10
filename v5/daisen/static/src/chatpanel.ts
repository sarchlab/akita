import { sendGetGitHubIsAvailable, sendPostGPT, GPTRequest, UnitContent } from "./chatpanelrequests";
import katex from "katex";
import "katex/dist/katex.min.css";
import html2canvas from "html2canvas";

type ChatContent = UnitContent[];


export class ChatPanel {
  _chatMessages: { role: "user" | "assistant" | "system"; content: ChatContent }[] = [];
  _uploadedFiles: { id: number; name: string; content: string; type: "file" | "image" | "image-screenshot"; size: string }[] = [];
  _fileUploadBtn: HTMLButtonElement;
  _imageUploadBtn: HTMLButtonElement;
  _screenshotUploadBtn: HTMLButtonElement;
  _fileIdCounter: number = 0;
  _screenshotIdCounter: number = 0;
  _fileListRow: HTMLDivElement = document.createElement("div");
  _attachRepoVisible: boolean = false;
  _attachRepoChecks: { [key: string]: boolean } = {};
  _uploadTraceVisible: boolean = false;
  _uploadTraceChecks: { [key: string]: boolean } = {};
  _githubIsAvailableResponse: { available: number; routine_keys: string[] } | null = null;
  _chatPanel: HTMLDivElement | null = null;
  _chatPanelWidth: number = 0;
  _traceAllComponentNames: string[] = [];
  _traceCurrentComponentNames: string[] = [];
  _traceStartTime: number;
  _traceEndTime: number;
  _traceSelectedStartTime: number;
  _traceSelectedEndTime: number;
  _tracePeriodUnitSwitch: boolean = true; // true for us, false for seconds
  _graphTestButtonClicked: boolean = false; // Flag to track if graphTest button was clicked
  _subpageTestButtonClicked: boolean = false; // Flag to track if subpageTest button was clicked

  // Chat history management
  _chatHistory: { 
    id: string; 
    title: string; 
    messages: { role: "user" | "assistant" | "system"; content: ChatContent }[];
    timestamp: number;
  }[] = [];
  _currentChatId: string = "";
  _chatIdCounter: number = 0;
  
  // Message navigation for current chat
  _messageNavigationIndex: number = -1; // Current position in navigation (-1 means not navigating)

  protected _onChatPanelOpen() {}
  protected _setTraceComponentNames() {}

  // Helper function to fetch and parse graphTest.csv
  async _fetchGraphTestCSV(): Promise<string[][]> {
    try {
      // Since the CSV fetch is returning HTML, let's embed the CSV data directly
      // This is the content from graphTest.csv in the src folder
      const csvText = `Component Class,Event Count
GPU.SA.CU.VALU,16064
GPU.SA.L1VAddrTrans,15936
GPU.SA.CU,13392
GPU.SA.L1VROB,10624
GPU.SA.L1IROB,6784
GPU.SA.L1VCache,5840
GPU.SA.CU.Scalar,5696
GPU.SA.L1VTLB,5348
GPU.SA.L1VCache.Local,5312
GPU.SA.L1SAddrTrans,3840
GPU.SA.L1ICache,3408
GPU.SA.L1ICache.Local,3392
GPU.SA.L1SROB,2560
GPU.SA.L1STLB,1292
GPU.SA.L1SCache,1292
GPU.SA.L1SCache.Local,1280
GPU.SA.CU.Special,1152
GPU.SA.CU.VMem,1088
GPU.SA.CU.Branch,1088
GPU.L2Cache,820
GPU.DRAM,531
GPU.DMA,275
GPU.L2TLB,65
GPU.SA.CU.WFPool,64
GPU.SA.L1IAddrTrans,48
GPU.SA.L1ITLB,20
Driver,16
GPU.CommandProcessor.Dispatcher0,16
MMU,13
GPU.CommandProcessor,9
`;
      
      // Parse CSV - simple parser for comma-separated values
      const lines = csvText.trim().split('\n');
      const data: string[][] = [];
      
      for (const line of lines) {
        // Split by comma and trim whitespace
        const cells = line.split(',').map(cell => cell.trim());
        data.push(cells);
      }
      
      return data;
    } catch (error) {
      console.error('Error processing CSV data:', error);
      throw error;
    }
  }

    async _csvStringToArray(csvText: string): Promise<string[][]> {
    try {
      // Parse CSV - simple parser for comma-separated values
      const lines = csvText.trim().split('\n');
      const data: string[][] = [];
      
      for (const line of lines) {
        // Split by comma and trim whitespace
        const cells = line.split(',').map(cell => cell.trim());
        data.push(cells);
      }
      
      return data;
    } catch (error) {
      console.error('Error processing CSV data:', error);
      throw error;
    }
  }

  // Helper function to convert CSV data to HTML table
  _csvToHTMLTable(csvData: string[][]): string {
    if (csvData.length === 0) {
      return '<p>No data available</p>';
    }

    let html = '<table style="border-collapse: collapse; width: 100%; margin: 10px 0;">';
    
    // Add header row if data exists
    if (csvData.length > 0) {
      html += '<thead><tr>';
      for (const header of csvData[0]) {
        html += `<th style="border: 1px solid #ddd; padding: 8px; background-color: #f2f2f2; text-align: left;">${header}</th>`;
      }
      html += '</tr></thead>';
    }
    
    // Add data rows
    if (csvData.length > 1) {
      html += '<tbody>';
      const maxRows = 10;
      const rowCount = csvData.length - 1;
      const showRows = Math.min(rowCount, maxRows);
      for (let i = 1; i <= showRows; i++) {
        html += '<tr>';
        for (const cell of csvData[i]) {
          html += `<td style="border: 1px solid #ddd; padding: 8px;">${cell}</td>`;
        }
        html += '</tr>';
      }
      if (rowCount > maxRows) {
        // Add ... row
        html += '<tr>';
        for (let j = 0; j < csvData[0].length; j++) {
          html += `<td style="border: 1px solid #ddd; padding: 8px;">...</td>`;
        }
        html += '</tr>';
      }
      html += '</tbody>';
    }
    
    html += '</table>';
    return html;
  }

  constructor() {
    this._fileIdCounter = 0;
    this._fileListRow = document.createElement("div");
    this._uploadedFiles = [];
    
    // Initialize with first chat
    this._createNewChat();
    
    sendGetGitHubIsAvailable().then((resp) => {
      this._githubIsAvailableResponse = resp;
    });
  }

  // Chat history management methods
  _createNewChat(): string {
    const chatId = `chat_${++this._chatIdCounter}`;
    const newChat = {
      id: chatId,
      title: "New Chat",
      messages: [{ role: "assistant" as const, content: [{ type: "text" as const, text: "Hello! What can I help you with today?" }] }],
      timestamp: Date.now()
    };
    
    // Save current chat if it exists and has user messages
    if (this._currentChatId && this._chatMessages.some(m => m.role === "user")) {
      this._saveChatToHistory();
    }
    
    this._chatHistory.push(newChat);
    this._currentChatId = chatId;
    this._chatMessages = [...newChat.messages];
    
    return chatId;
  }

  _saveChatToHistory(): void {
    if (!this._currentChatId) return;
    
    const chatIndex = this._chatHistory.findIndex(c => c.id === this._currentChatId);
    if (chatIndex !== -1) {
      this._chatHistory[chatIndex].messages = [...this._chatMessages];
      this._chatHistory[chatIndex].timestamp = Date.now();
      
      // Update title based on first user message
      const firstUserMessage = this._chatMessages.find(m => m.role === "user");
      if (firstUserMessage) {
        if (firstUserMessage.content[0].type === "text") {
          const words = firstUserMessage.content[0].text.split(" ").slice(0, 6);
          this._chatHistory[chatIndex].title = words.join(" ") + (words.length === 6 ? "..." : "");
        } else {
          this._chatHistory[chatIndex].title = "Unknown Title";
        }

        // const words = firstUserMessage.content[0].text.split(" ").slice(0, 6);
        // this._chatHistory[chatIndex].title = words.join(" ") + (words.length === 6 ? "..." : "");
      }
    }
  }

  _loadChat(chatId: string): void {
    // Save current chat first
    this._saveChatToHistory();
    
    const chat = this._chatHistory.find(c => c.id === chatId);
    if (chat) {
      this._currentChatId = chatId;
      this._chatMessages = [...chat.messages];
    }
  }

  _deleteChat(chatId: string): void {
    this._chatHistory = this._chatHistory.filter(c => c.id !== chatId);
    
    if (this._currentChatId === chatId) {
      if (this._chatHistory.length > 0) {
        // Load the most recent chat
        const mostRecent = this._chatHistory.reduce((latest, chat) => 
          chat.timestamp > latest.timestamp ? chat : latest, this._chatHistory[0]
        );
        this._loadChat(mostRecent.id);
      } else {
        // Create a new chat if no history exists
        this._createNewChat();
      }
    }
  }

  // Message navigation methods
  _getCurrentChatUserMessages(): string[] {
    // Extract user messages from current chat (excluding system messages and file prefixes)
    const currentChat = this._chatHistory.find(c => c.id === this._currentChatId);
    if (!currentChat) return [];

    return currentChat.messages
      .filter(m => m.role === "user")
      .map(m => {
        const firstContent = m.content[0];
        if (firstContent.type === "text") {
          // Remove uploaded files prefix if present
          const idx = firstContent.text.indexOf("[End Uploaded Files]");
          if (idx !== -1) {
            let after = firstContent.text.slice(idx + "[End Uploaded Files]".length);
            if (after.startsWith("\n")) after = after.slice(1);
            return after;
          }
          return firstContent.text;
        }
        // If not text, skip or return empty string
        return "";
      })
      .filter(content => content.trim().length > 0); // Remove empty messages
  }

  _navigateMessageHistory(direction: "up" | "down", inputElement: HTMLTextAreaElement): void {
    const userMessages = this._getCurrentChatUserMessages();
    if (userMessages.length === 0) return;

    if (direction === "up") {
      // Go back in history (older messages)
      if (this._messageNavigationIndex === -1) {
        // First time navigating - start from the most recent message
        this._messageNavigationIndex = userMessages.length - 1;
      } else if (this._messageNavigationIndex > 0) {
        this._messageNavigationIndex--;
      }
    } else if (direction === "down") {
      // Go forward in history (newer messages)
      if (this._messageNavigationIndex !== -1) {
        this._messageNavigationIndex++;
        if (this._messageNavigationIndex >= userMessages.length) {
          // Reached the end, clear input
          this._messageNavigationIndex = -1;
          inputElement.value = "";
          inputElement.style.height = "38px";
          return;
        }
      }
    }

    // Set the input value and adjust height
    if (this._messageNavigationIndex !== -1 && this._messageNavigationIndex < userMessages.length) {
      inputElement.value = userMessages[this._messageNavigationIndex];
      // Trigger the auto-resize
      inputElement.style.height = "auto";
      inputElement.style.height = (inputElement.scrollHeight) + "px";
    }
  }

  _showChatPanel() {
    // let messages: { role: "user" | "assistant" | "system"; content: string }[] = [
    //   { role: "system", content: "You are Daisen Bot."}
    // ];
    let messages = this._chatMessages;
    this._injectChatPanelCSS();

    // Check if panel already exists
    let chatPanel = document.getElementById("chat-panel") as HTMLElement;
    let isNewPanel = false;
    
    if (chatPanel) {
      // Clear existing content but preserve the panel structure
      const existingContent = chatPanel.querySelector('.chat-content');
      if (existingContent) {
        existingContent.remove();
      }
    } else {
      isNewPanel = true;
      // Create the chat panel
      chatPanel = document.createElement("div");
      chatPanel.id = "chat-panel";
      chatPanel.style.position = "fixed";
      chatPanel.style.right = "0";
      chatPanel.style.width = "600px";
      chatPanel.style.background = "rgba(255,255,255,0.7)";
      chatPanel.style.zIndex = "9999";
      chatPanel.style.boxShadow = "0 0 10px rgba(0,0,0,0.2)";
      chatPanel.style.display = "flex";
      chatPanel.style.flexDirection = "column";
      chatPanel.style.justifyContent = "flex-start";
      chatPanel.style.overflow = "hidden";

      chatPanel.addEventListener("dragover", (e) => {
        e.preventDefault();
        chatPanel.style.background = "rgba(220,240,255,0.7)";
      });
      chatPanel.addEventListener("dragleave", (e) => {
        e.preventDefault();
        chatPanel.style.background = "rgba(255,255,255,0.7)";
      });
      chatPanel.addEventListener("drop", (e) => {
        e.preventDefault();
        chatPanel.style.background = "rgba(255,255,255,0.7)";
        const files = e.dataTransfer?.files;
        if (files && files.length > 0) {
          handleFileUpload(files[0]);
        }
      });

      document.body.appendChild(chatPanel);

      // Set chat panel height and top to match #inner-container
      const innerContainer = document.getElementById("inner-container");
      if (innerContainer) {
        const rect = innerContainer.getBoundingClientRect();
        chatPanel.style.top = rect.top + "px";
        chatPanel.style.height = rect.height + "px";
      } else {
        // fallback to full viewport height if not found
        chatPanel.style.top = "0";
        chatPanel.style.height = "100vh";
      }
    }

    // Get and update chat panel width after it's added to DOM (only for new panels)
    if (isNewPanel) {
      setTimeout(() => {
        this._getChatPanelWidth();
      }, 10);

      // Force reflow to ensure the browser registers the new height before animating
      void chatPanel.offsetHeight;

      // Animate in
      chatPanel.classList.add('open');
      this._onChatPanelOpen();
    }
    // this._showChatButton = false;
    // this._addPaginationControl();

    // // Store the original width before shrinking
    // const canvasContainer = this._canvas;
    // let originalCanvasWidth = "";
    // if (canvasContainer) {
    //   originalCanvasWidth = canvasContainer.style.width;
    //   canvasContainer.style.transition = "width 0.3s cubic-bezier(.4,0,.2,1)";
    //   canvasContainer.style.width = "calc(100% - 600px)";
    //   setTimeout(() => {
    //     this._resize();
    //     this._renderPage();
    //   }, 300);
    // }

    const chatContent = document.createElement("div");
    chatContent.className = "chat-content";
    chatContent.style.flex = "1";
    chatContent.style.display = "flex";
    chatContent.style.flexDirection = "column";
    chatContent.style.padding = "20px";
    chatContent.style.minHeight = "0";
    chatPanel.appendChild(chatContent);
    
    this._chatPanel = chatPanel as HTMLDivElement;

    // Top bar with chat history dropdown and New Chat button
    const topBar = document.createElement("div");
    topBar.style.display = "flex";
    topBar.style.justifyContent = "space-between";
    topBar.style.alignItems = "center";
    topBar.style.marginBottom = "10px";
    topBar.style.minHeight = "32px";
    chatContent.appendChild(topBar);

    // Chat history dropdown container
    const chatHistoryContainer = document.createElement("div");
    chatHistoryContainer.style.position = "relative";
    chatHistoryContainer.style.display = "flex";
    chatHistoryContainer.style.alignItems = "center";
    chatHistoryContainer.style.gap = "8px";
    
    // Chat history dropdown
    const chatHistorySelect = document.createElement("select");
    chatHistorySelect.style.padding = "6px";
    chatHistorySelect.style.borderRadius = "4px";
    chatHistorySelect.style.border = "1px solid #ccc";
    chatHistorySelect.style.background = "#fff";
    chatHistorySelect.style.fontSize = "16px";
    chatHistorySelect.style.width = "150px";
    chatHistorySelect.style.height = "38px";
    
    // Function to update the dropdown options
    const updateChatHistoryDropdown = () => {
      chatHistorySelect.innerHTML = "";
      
      // Sort chats by timestamp (most recent first)
      const sortedChats = [...this._chatHistory].sort((a, b) => b.timestamp - a.timestamp);
      
      sortedChats.forEach(chat => {
        const option = document.createElement("option");
        option.value = chat.id;
        option.textContent = chat.title;
        option.selected = chat.id === this._currentChatId;
        chatHistorySelect.appendChild(option);
      });
      
      // Add "No chats" option if empty
      if (this._chatHistory.length === 0) {
        const option = document.createElement("option");
        option.textContent = "No chat history";
        option.disabled = true;
        chatHistorySelect.appendChild(option);
      }
    };
    
    // Handle chat selection
    chatHistorySelect.onchange = () => {
      const selectedChatId = chatHistorySelect.value;
      if (selectedChatId && selectedChatId !== this._currentChatId) {
        this._loadChat(selectedChatId);
        // Re-render the panel to show the selected chat
        this._showChatPanel();
      }
    };
    
    // Delete button for current chat
    const deleteChatBtn = document.createElement("button");
    // deleteChatBtn.textContent = "×";
    deleteChatBtn.title = "Delete selected chat";
    deleteChatBtn.style.background = "#dc3545";
    deleteChatBtn.style.color = "#fff";
    deleteChatBtn.style.border = "none";
    deleteChatBtn.style.borderRadius = "6px";
    deleteChatBtn.style.width = "38px";
    deleteChatBtn.style.height = "38px";
    // deleteChatBtn.style.fontSize = "24px";
    deleteChatBtn.style.cursor = "pointer";
    deleteChatBtn.style.display = "flex";
    deleteChatBtn.style.alignItems = "center";
    deleteChatBtn.style.justifyContent = "center";
    deleteChatBtn.innerHTML = `
      <svg class="svg-icon" style="width: 24px; height: 24px; vertical-align: middle; fill: currentColor; overflow: hidden;" viewBox="0 0 1024 1024" xmlns="http://www.w3.org/2000/svg">
        <path d="M262.2 304.9h-4.8c0.8-0.1 1.6-0.1 2.4-0.1 0.8 0 1.6 0 2.4 0.1z" fill="#fff"/>
        <path d="M589.4 358.4c0 15.2 12.3 27.5 27.4 27.5h25.8c15.2 0 27.5-12.3 27.5-27.4 0-15.2-12.3-27.4-27.5-27.4h-25.8c-15.1-0.1-27.4 12.2-27.4 27.3zM616.3 850.4c15.2 0.6 27.9-11.3 28.4-26.4l0.9-351c0.6-15.2-11.3-27.9-26.4-28.4-15.2-0.6-27.9 11.3-28.4 26.4l-0.9 351c-0.6 15.1 11.3 27.9 26.4 28.4zM457.1 822l-0.9-351c-0.6-15.2-13.3-27-28.4-26.4-15.1 0.6-27 13.3-26.4 28.4l0.9 351c0.6 15.1 13.3 27 28.4 26.4 15.1-0.5 27-13.3 26.4-28.4z" fill="#fff"/>
        <path d="M826.5 358.3l-1.7 27.6-27.9 502.4-0.4 6.9c0 24.5-19.6 44.4-43.9 45H272.1c-24.3-0.6-43.9-20.6-43.9-45l-0.4-6.8L200 385.9l-1.7-27.6v-0.3c0.2-14.2 11.1-25.8 25.1-27H518c15.2 0 27.5 12.3 27.5 27.4 0 15.2-12.3 27.4-27.5 27.4H255l22.5 405.8 4.9 79.3 0.5 7.9 0.4 6.7h458.4l0.4-6.7 0.5-7.9 4.9-79.3L770 385.8h-28.3c-15.2 0-27.4-12.3-27.4-27.4 0-15.2 12.3-27.4 27.4-27.4h59.8c14 1.2 25 12.9 25.1 27.1-0.1 0.1-0.1 0.1-0.1 0.2zM475.2 143.2l-4.6-27c-2.5-14.9 7.5-29.1 22.5-31.7C508 82 522.2 92 524.8 107l4.6 27c2.5 14.9-7.5 29.1-22.5 31.7-15 2.5-29.2-7.5-31.7-22.5z" fill="#fff"/>
        <path d="M792.6 150.4l-560.5 95.4c-14.9 2.5-29.1-7.5-31.7-22.5-2.5-14.9 7.5-29.1 22.5-31.7l560.5-95.4c14.9-2.5 29.1 7.5 31.7 22.5 2.5 14.9-7.5 29.1-22.5 31.7z" fill="#fff"/>
      </svg>
    `;
    deleteChatBtn.onclick = () => {
      if (confirm("Are you sure you want to delete this chat?")) {
        this._deleteChat(this._currentChatId);
        this._showChatPanel(); // Re-render after deletion
      }
    };
    
    chatHistoryContainer.appendChild(chatHistorySelect);
    chatHistoryContainer.appendChild(deleteChatBtn);
    topBar.appendChild(chatHistoryContainer);

    // Create New Chat button for top bar
    const newChatBtn = document.createElement("button");
    newChatBtn.title = "New chat";
    // newChatBtn.className = "btn btn-secondary";
    newChatBtn.style.flexShrink = "0";

    newChatBtn.style.display = "flex";
    newChatBtn.style.alignItems = "center";
    newChatBtn.style.justifyContent = "center";
    newChatBtn.style.width = "38px";
    newChatBtn.style.height = "38px";
    newChatBtn.style.borderRadius = "6px";
    newChatBtn.style.background = "#0d6efd";
    newChatBtn.style.border = "none";
    newChatBtn.style.cursor = "pointer";
    newChatBtn.innerHTML = `
      <svg class="svg-icon" style="width: 24px; height: 24px; vertical-align: middle; fill: currentColor; overflow: hidden;" viewBox="0 0 20 20" xmlns="http://www.w3.org/2000/svg">
        <path d="M2.6687 11.333V8.66699C2.6687 7.74455 2.66841 7.01205 2.71655 6.42285C2.76533 5.82612 2.86699 5.31731 3.10425 4.85156L3.25854 4.57617C3.64272 3.94975 4.19392 3.43995 4.85229 3.10449L5.02905 3.02149C5.44666 2.84233 5.90133 2.75849 6.42358 2.71582C7.01272 2.66769 7.74445 2.66797 8.66675 2.66797H9.16675C9.53393 2.66797 9.83165 2.96586 9.83179 3.33301C9.83179 3.70028 9.53402 3.99805 9.16675 3.99805H8.66675C7.7226 3.99805 7.05438 3.99834 6.53198 4.04102C6.14611 4.07254 5.87277 4.12568 5.65601 4.20313L5.45581 4.28906C5.01645 4.51293 4.64872 4.85345 4.39233 5.27149L4.28979 5.45508C4.16388 5.7022 4.08381 6.01663 4.04175 6.53125C3.99906 7.05373 3.99878 7.7226 3.99878 8.66699V11.333C3.99878 12.2774 3.99906 12.9463 4.04175 13.4688C4.08381 13.9833 4.16389 14.2978 4.28979 14.5449L4.39233 14.7285C4.64871 15.1465 5.01648 15.4871 5.45581 15.7109L5.65601 15.7969C5.87276 15.8743 6.14614 15.9265 6.53198 15.958C7.05439 16.0007 7.72256 16.002 8.66675 16.002H11.3337C12.2779 16.002 12.9461 16.0007 13.4685 15.958C13.9829 15.916 14.2976 15.8367 14.5447 15.7109L14.7292 15.6074C15.147 15.3511 15.4879 14.9841 15.7117 14.5449L15.7976 14.3447C15.8751 14.128 15.9272 13.8546 15.9587 13.4688C16.0014 12.9463 16.0017 12.2774 16.0017 11.333V10.833C16.0018 10.466 16.2997 10.1681 16.6667 10.168C17.0339 10.168 17.3316 10.4659 17.3318 10.833V11.333C17.3318 12.2555 17.3331 12.9879 17.2849 13.5771C17.2422 14.0993 17.1584 14.5541 16.9792 14.9717L16.8962 15.1484C16.5609 15.8066 16.0507 16.3571 15.4246 16.7412L15.1492 16.8955C14.6833 17.1329 14.1739 17.2354 13.5769 17.2842C12.9878 17.3323 12.256 17.332 11.3337 17.332H8.66675C7.74446 17.332 7.01271 17.3323 6.42358 17.2842C5.90135 17.2415 5.44665 17.1577 5.02905 16.9785L4.85229 16.8955C4.19396 16.5601 3.64271 16.0502 3.25854 15.4238L3.10425 15.1484C2.86697 14.6827 2.76534 14.1739 2.71655 13.5771C2.66841 12.9879 2.6687 12.2555 2.6687 11.333ZM13.4646 3.11328C14.4201 2.334 15.8288 2.38969 16.7195 3.28027L16.8865 3.46485C17.6141 4.35685 17.6143 5.64423 16.8865 6.53613L16.7195 6.7207L11.6726 11.7686C11.1373 12.3039 10.4624 12.6746 9.72827 12.8408L9.41089 12.8994L7.59351 13.1582C7.38637 13.1877 7.17701 13.1187 7.02905 12.9707C6.88112 12.8227 6.81199 12.6134 6.84155 12.4063L7.10132 10.5898L7.15991 10.2715C7.3262 9.53749 7.69692 8.86241 8.23218 8.32715L13.2791 3.28027L13.4646 3.11328ZM15.7791 4.2207C15.3753 3.81702 14.7366 3.79124 14.3035 4.14453L14.2195 4.2207L9.17261 9.26856C8.81541 9.62578 8.56774 10.0756 8.45679 10.5654L8.41772 10.7773L8.28296 11.7158L9.22241 11.582L9.43433 11.543C9.92426 11.432 10.3749 11.1844 10.7322 10.8271L15.7791 5.78027L15.8552 5.69629C16.185 5.29194 16.1852 4.708 15.8552 4.30371L15.7791 4.2207Z" fill="#fff"/>
      </svg>
    `;

    newChatBtn.onclick = () => {
      // Check if current chat is already new/empty (only has initial greeting, no user messages)
      const hasUserMessages = this._chatMessages.some(m => m.role === "user");
      const hasOnlyGreeting = this._chatMessages.length === 1 && 
                              this._chatMessages[0].role === "assistant" && 
                              this._chatMessages[0].content[0].type === "text" &&
                              this._chatMessages[0].content[0].text === "Hello! What can I help you with today?";
      
      // Do nothing if chat is already new/empty
      if (!hasUserMessages && (this._chatMessages.length === 0 || hasOnlyGreeting)) {
        return;
      }
      
      this._createNewChat();
      // this._onChatPanelOpen();
      this._showChatPanel(); // Re-render the panel with the new chat
    };
    topBar.appendChild(newChatBtn);
    
    // Update dropdown with current data
    updateChatHistoryDropdown();
    
    // Reset message navigation index for current chat
    this._messageNavigationIndex = -1;

    // Add separator bar between top controls and chat content
    const separatorBar = document.createElement("div");
    separatorBar.style.width = "100%";
    separatorBar.style.height = "1px";
    separatorBar.style.backgroundColor = "rgb(204, 204, 204)";
    separatorBar.style.marginTop = "6px";
    separatorBar.style.marginBottom = "8px";
    separatorBar.style.marginBottom = "10px";
    chatContent.appendChild(separatorBar);

    // Message display area
    const messagesDiv = document.createElement("div");
    messagesDiv.style.flex = "1 1 0%";
    messagesDiv.style.height = "0";
    messagesDiv.style.overflowY = "auto";
    messagesDiv.style.marginBottom = "10px";
    messagesDiv.style.background = "rgba(255, 255, 255, 0.5)";
    messagesDiv.style.borderRadius = "6px";
    messagesDiv.style.padding = "8px";
    chatContent.appendChild(messagesDiv);

    // Loading messages
    messages
      .filter(m => m.role !== "system")
      .forEach(m => {
        if (m.role === "user") {
          const userDiv = document.createElement("div");
          userDiv.style.display = "flex";
          userDiv.style.flexDirection = "column";
          userDiv.style.alignItems = "flex-end";
          userDiv.style.margin = "4px 0";

          const userBubble = document.createElement("span");
          const firstContent = m.content[0];
          const userText = firstContent && firstContent.type === "text" ? firstContent.text : "Unable to display message";
          userBubble.innerHTML = "<b>You:</b> " + userText;
          userBubble.style.background = "#0d6efd";
          userBubble.style.color = "white";
          userBubble.style.padding = "8px 12px";
          userBubble.style.borderRadius = "16px";
          userBubble.style.maxWidth = "90%";
          userBubble.style.display = "inline-block";
          userBubble.style.wordBreak = "break-word";
          userDiv.appendChild(userBubble);

          // Add URL underneath the user bubble
          const urlDiv = document.createElement("div");
          urlDiv.innerHTML = this._getCurrentFrontendURL();
          urlDiv.style.fontSize = "10px";
          urlDiv.style.color = "#999999";
          urlDiv.style.marginTop = "2px";
          urlDiv.style.textAlign = "right";
          urlDiv.style.maxWidth = "90%";
          urlDiv.style.wordBreak = "break-all";
          userDiv.appendChild(urlDiv);

          messagesDiv.appendChild(userDiv);
        } else if (m.role === "assistant") {
          const botDiv = document.createElement("div");
          const firstContent = m.content[0];
          if (firstContent.type === "text") {
            botDiv.innerHTML = "<b>Daisen Bot:</b> " + convertMarkdownToHTML(autoWrapMath(firstContent.text));
          } else if (firstContent.type === "image_url") {
            botDiv.innerHTML = "<b>Daisen Bot:</b> " + "Something went wrong, I can't display history right now.";
          }
          botDiv.style.textAlign = "left";
          botDiv.style.margin = "4px 0";
          messagesDiv.appendChild(botDiv);
          
        }
      });
    // apply KaTeX rendering for math
    messagesDiv.querySelectorAll('.math').forEach(el => {
      try {
        const tex = el.textContent || "";
        const displayMode = el.getAttribute("data-display") === "block";
        console.log("Rendering math:", tex, "Display mode:", displayMode);
        el.innerHTML = katex.renderToString(tex, { displayMode });
      } catch (e) {
        el.innerHTML = "<span style='color:red'>Invalid math</span>";
        console.log("KaTeX error:", e, "for tex:", el.textContent);
      }
    });

    // Show welcome message only if there are no messages (new empty chat)
    const hasMessages = messages.some(m => m.role === "user" || m.role === "assistant");
    if (!hasMessages) {
      const welcomeDiv = document.createElement("div");
      welcomeDiv.innerHTML = "<b>Daisen Bot:</b> Hello! What can I help you with today?";
      welcomeDiv.style.textAlign = "left";
      welcomeDiv.style.marginBottom = "8px";
      messagesDiv.appendChild(welcomeDiv);
    }

    // File list container (above upload button row)
    const fileListRow = document.createElement("div");
    fileListRow.style.display = "flex";
    fileListRow.style.flexDirection = "column";
    fileListRow.style.gap = "4px";
    chatContent.appendChild(fileListRow);

    // Make it accessible to renderFileList
    this._fileListRow = fileListRow;

    // Action buttons row (above input)
    const actionRow = document.createElement("div");
    actionRow.style.display = "flex";
    actionRow.style.gap = "8px";
    actionRow.style.marginBottom = "8px";

    // File upload functionality
    const handleFileUpload = (file: File) => {
      const allowed = [".sqlite", ".sqlite3", ".csv", ".txt", ".json", ".py", ".js", ".c", ".cpp", ".java"];
      const name = file.name.toLowerCase();
      const validSuffix = allowed.some(suffix => name.endsWith(suffix));
      if (!validSuffix) {
        window.alert("Invalid file type. Allowed: .sqlite, .sqlite3, .csv, .txt, .json, .py, .js, .c, .cpp, .java");
        return;
      }

      // Check size
      if (file.size > 32 * 1024) {
        window.alert("File too large. Max size is 32 KB.");
        return;
      }

      // Read file
      const reader = new FileReader();
      reader.onload = (e) => {
        const sizeStr = formatFileSize(file.size);
        this._uploadedFiles.push({
          id: ++this._fileIdCounter,
          name: file.name,
          content: e.target?.result as string,
          type: "file",
          size: sizeStr,
        });
        renderFileList.call(this);
      };
      reader.readAsText(file);
    };

    // File upload button
    const fileUploadBtn = document.createElement("button");
    fileUploadBtn.type = "button";
    fileUploadBtn.title = "Upload File";
    fileUploadBtn.style.background = "#f6f8fa";
    fileUploadBtn.style.border = "1px solid #ccc";
    fileUploadBtn.style.borderRadius = "6px";
    fileUploadBtn.style.width = "38px";
    fileUploadBtn.style.height = "38px";
    fileUploadBtn.style.display = "flex";
    fileUploadBtn.style.alignItems = "center";
    fileUploadBtn.style.justifyContent = "center";
    fileUploadBtn.style.cursor = "pointer";
    fileUploadBtn.innerHTML = `
      <svg width="24" height="24" viewBox="0 0 20 20" fill="currentColor">
        <path d="m 14.3352 17.5003 v -1.8349 h -1.835 c -0.3671 0 -0.6648 -0.298 -0.665 -0.6651 c 0 -0.3672 0.2978 -0.665 0.665 -0.665 h 1.835 v -1.835 c 0 -0.3672 0.2978 -0.665 0.665 -0.665 c 0.3672 0.0002 0.6651 0.2979 0.6651 0.665 v 1.835 h 1.8349 l 0.1338 0.0137 c 0.303 0.062 0.5313 0.33 0.5313 0.6513 c -0.0002 0.3212 -0.2284 0.5894 -0.5313 0.6514 l -0.1338 0.0137 h -1.8349 v 1.8349 c -0.0002 0.367 -0.298 0.6649 -0.6651 0.6651 c -0.3671 0 -0.6648 -0.298 -0.665 -0.6651 z m 1.666 -9.167 v -1 c 0 -0.711 0.0001 -1.2044 -0.0312 -1.5879 c -0.0231 -0.282 -0.0609 -0.4715 -0.1123 -0.6152 l -0.0557 -0.1299 c -0.1539 -0.3021 -0.3883 -0.5551 -0.6758 -0.7314 l -0.1269 -0.0703 c -0.158 -0.0804 -0.3696 -0.1373 -0.7451 -0.168 c -0.3835 -0.0313 -0.877 -0.0322 -1.5879 -0.0322 h -3.502 c 0.0001 0.048 -0.0048 0.0967 -0.0156 0.1455 l -0.5322 -0.1182 h -0.001 l 0.5332 0.1182 l -0.4561 2.0557 c -0.1052 0.4736 -0.1851 0.8633 -0.3535 1.1934 l -0.0771 0.1377 c -0.1388 0.222 -0.3147 0.4178 -0.5186 0.5801 l -0.2129 0.1514 c -0.2687 0.1679 -0.5789 0.2582 -0.9453 0.3438 l -0.3857 0.0869 l -2.0557 0.4561 l -0.1182 -0.5332 v 0.001 l 0.1182 0.5322 c -0.0488 0.0108 -0.0975 0.0157 -0.1455 0.0156 v 3.5019 c 0 0.7109 0.0009 1.2044 0.0322 1.5879 c 0.0307 0.3756 0.0875 0.5871 0.168 0.7451 l 0.0703 0.127 c 0.1763 0.2874 0.4293 0.5218 0.7314 0.6758 l 0.1299 0.0556 c 0.1438 0.0515 0.3333 0.0893 0.6152 0.1123 c 0.3835 0.0314 0.8769 0.0313 1.5879 0.0313 h 0.9512 c 0.3672 0 0.6649 0.2979 0.665 0.665 c 0 0.3673 -0.2978 0.6651 -0.665 0.6651 h -0.9512 c -0.6891 0 -1.246 0.0006 -1.6963 -0.0362 c -0.4005 -0.0327 -0.7614 -0.097 -1.0976 -0.2412 l -0.1426 -0.0674 c -0.5211 -0.2655 -0.9576 -0.6692 -1.2617 -1.165 l -0.1221 -0.2178 c -0.192 -0.3767 -0.2712 -0.7832 -0.3086 -1.2412 c -0.0368 -0.4502 -0.0361 -1.0073 -0.0361 -1.6963 v -3.666 c 0 -1.5596 0.921 -3.1377 2.0938 -4.2939 c 1.1711 -1.1547 2.7465 -2.0389 4.2383 -2.0371 v -0.001 h 3.6661 c 0.689 0 1.246 -0.0006 1.6962 0.0361 c 0.4581 0.0374 0.8645 0.1166 1.2413 0.3086 l 0.2177 0.1221 c 0.4959 0.3041 0.8996 0.7406 1.1651 1.2617 l 0.0664 0.1426 c 0.1443 0.3363 0.2094 0.697 0.2422 1.0977 c 0.0368 0.4502 0.0361 1.0072 0.0361 1.6963 v 1 c 0 0.3673 -0.2978 0.665 -0.665 0.665 c -0.3672 -0.0002 -0.6651 -0.2979 -0.6651 -0.665 z m -8.2412 -4.0713 c -0.6983 0.277 -1.4238 0.76 -2.0644 1.3916 c -0.6534 0.6443 -1.1579 1.3829 -1.4414 2.1084 l 1.6572 -0.3682 l 0.3955 -0.0899 c 0.3187 -0.0757 0.4299 -0.1145 0.5186 -0.1699 l 0.0898 -0.0635 c 0.0861 -0.0685 0.1601 -0.1514 0.2188 -0.2451 l 0.0518 -0.1035 c 0.0503 -0.1242 0.1018 -0.3326 0.208 -0.8106 l 0.3662 -1.6494 z" />
      </svg>
    `;

    // Hidden file input
    const fileInput = document.createElement("input");
    fileInput.type = "file";
    fileInput.style.display = "none";
    fileInput.accept = ".sqlite,.sqlite3,.csv,.txt,.json,.py,.js,.c,.cpp,.java";
    fileUploadBtn.onclick = () => fileInput.click();

    fileInput.onchange = () => {
      const file = fileInput.files?.[0];
      if (file) handleFileUpload(file);
    };

    actionRow.appendChild(fileUploadBtn);
    actionRow.appendChild(fileInput);

    this._fileUploadBtn = fileUploadBtn;






    // Image upload Button

    // File upload functionality
    const handleImageUpload = (file: File) => {
      const allowed = [".png", ".jpg", ".jpeg"];
      const name = file.name.toLowerCase();
      const validSuffix = allowed.some(suffix => name.endsWith(suffix));
      if (!validSuffix) {
        window.alert("Invalid file type. Allowed: .png, .jpg, .jpeg");
        return;
      }

      // Check size
      if (file.size > 256 * 1024) {
        window.alert("File too large. Max size is 256 KB.");
        return;
      }

      // Read image file
      const reader = new FileReader();
      reader.onload = (e) => {
        // console.log("Image file loaded:", file.name, e.target?.result);
        const sizeStr = formatFileSize(file.size);
        this._uploadedFiles.push({
          id: ++this._fileIdCounter,
          name: file.name,
          content: e.target?.result as string,
          type: "image",
          size: sizeStr,
        });
        renderFileList.call(this);
      };
      reader.readAsDataURL(file);
    };

    // Image upload button
    const imageUploadBtn = document.createElement("button");
    imageUploadBtn.type = "button";
    imageUploadBtn.title = "Upload Image";
    imageUploadBtn.style.background = "#f6f8fa";
    imageUploadBtn.style.border = "1px solid #ccc";
    imageUploadBtn.style.borderRadius = "6px";
    imageUploadBtn.style.width = "38px";
    imageUploadBtn.style.height = "38px";
    imageUploadBtn.style.display = "flex";
    imageUploadBtn.style.alignItems = "center";
    imageUploadBtn.style.justifyContent = "center";
    imageUploadBtn.style.cursor = "pointer";
    imageUploadBtn.innerHTML = `
      <svg width="24" height="24" viewBox="0 0 64 64" fill="currentColor">
        <path d="M25 20.775c0-2.325-1.9-4.225-4.225-4.225-2.325 0-4.225 1.9-4.225 4.225 0 2.325 1.9 4.225 4.225 4.225S25 23.1 25 20.775zm14.025 21.55H42.5V38.85c0-2.575 1.55-4.775 3.75-5.775l-.575-2v-.15l-3.8-11.525-13.325 17.175-3.575-6.05L10.825 44.8h20.85 2.35c1.125-1.5 2.95-2.475 5-2.475zM61.5 11.95c0-5.2-4.25-9.45-9.45-9.45H11.95c-5.2 0-9.45 4.25-9.45 9.45v40.125c0 5.175 4.25 9.425 9.45 9.425h28.3s0 0 0 0c.975 0 1.775-.8 1.775-1.775s-.8-1.775-1.775-1.775c0 0 0 0 0 0H11.95c-3.25 0-5.9-2.65-5.9-5.9V11.95c0-3.25 2.65-5.9 5.9-5.9h40.125c3.25 0 5.9 2.65 5.9 5.9V40.1c0 .025 0 .025 0 .05 0 .975.8 1.775 1.775 1.775s1.775-.8 1.775-1.775c0 0 0 0 0-.025h.025V11.95H61.5zM50 59.4509v-5.5047h-5.505c-1.1013 0-1.9944-.894-1.995-1.9953 0-1.1016.8934-1.995 1.995-1.995H50v-5.505c0-1.1016.8934-1.995 1.995-1.995 1.1016.0006 1.9953.8937 1.9953 1.995v5.505h5.5047l.4014.0411c.909.186 1.5939.99 1.5939 1.9539-.0006.9636-.6852 1.7682-1.5939 1.9542l-.4014.0411h-5.5047v5.5047c-.0006 1.101-.894 1.9947-1.9953 1.9953-1.1013 0-1.9944-.894-1.995-1.9953z" />
      </svg>
    `;

    // Hidden image input
    const imageInput = document.createElement("input");
    imageInput.type = "file";
    imageInput.style.display = "none";
    imageInput.accept = ".png,.jpg,.jpeg";
    imageUploadBtn.onclick = () => imageInput.click();

    imageInput.onchange = () => {
      const file = imageInput.files?.[0];
      if (file) handleImageUpload(file);
    };

    actionRow.appendChild(imageUploadBtn);
    actionRow.appendChild(imageInput);
    

    this._imageUploadBtn = imageUploadBtn;




    // Screenshot upload button
    const screenshotUploadBtn = document.createElement("button");
    screenshotUploadBtn.type = "button";
    screenshotUploadBtn.title = "Upload Screenshot";
    screenshotUploadBtn.style.background = "#f6f8fa";
    screenshotUploadBtn.style.border = "1px solid #ccc";
    screenshotUploadBtn.style.borderRadius = "6px";
    screenshotUploadBtn.style.width = "38px";
    screenshotUploadBtn.style.height = "38px";
    screenshotUploadBtn.style.display = "flex";
    screenshotUploadBtn.style.alignItems = "center";
    screenshotUploadBtn.style.justifyContent = "center";
    screenshotUploadBtn.style.cursor = "pointer";
    screenshotUploadBtn.innerHTML = `
      <svg width="24" height="24" viewBox="0 0 21 21" fill="currentColor">
        <path d="M4 19h3v-2H5v-2H3v3a1 1 0 001 1zM19 4a1 1 0 00-1-1h-3v2h2v2h2V4zM5 5h2V3H4A1 1 0 003 4v3h2V5zM3 9h2v4H3zm14 0h2v3h-2zM9 3h4v2H9zm0 14h3v2H9z M15.9469 18.9611v-1.7557h-1.7558c-.3512 0-.6361-.2851-.6363-.6364 0-.3513.2849-.6363.6363-.6363H15.9469v-1.7558c0-.3513.2849-.6363.6363-.6363.3513.0002.6364.285.6364.6363v1.7558h1.7557l.128.0131c.2899.0593.5084.3157.5084.6232-.0002.3073-.2185.5639-.5084.6233l-.128.0131h-1.7557v1.7557c-.0002.3512-.2851.6362-.6364.6364-.3512 0-.6361-.2851-.6363-.6364z" />
      </svg>
    `;

    screenshotUploadBtn.onclick = async () => {
      const innerContainer = document.body; //document.getElementById("container");
      if (!innerContainer) {
        alert("No inner-container found!");
        return;
      }

      // Capture as PNG
      const canvas = await html2canvas(innerContainer);

      const img = document.createElement("img");
      img.src = canvas.toDataURL("image/png");
      img.style.position = "fixed";
      img.style.left = innerContainer.getBoundingClientRect().left + "px";
      img.style.top = innerContainer.getBoundingClientRect().top + "px";
      img.style.width = innerContainer.offsetWidth + "px";
      img.style.height = innerContainer.offsetHeight + "px";
      img.style.zIndex = "99999";
      img.style.transition = "all 1.2s cubic-bezier(.4,0,.2,1), opacity 0.5s";
      img.style.border = "10px solid #fff";
      img.style.borderRadius = "10px";
      img.style.boxShadow = "0 8px 32px rgba(0,0,0,0.25), 0 2px 8px rgba(0,0,0,0.10)";
      document.body.appendChild(img);

      // Start animation after a tick
      const imgWidth = 400;
      const imgHeight = innerContainer.offsetHeight * (imgWidth / innerContainer.offsetWidth);
      console.log("Image dimensions:", img.naturalWidth, img.naturalHeight, "Scaled height:", imgHeight);
      
      setTimeout(() => {
        img.style.left = `calc(100vw - ${imgWidth + 20}px)`; //"20px";
        img.style.top = `calc(100vh - ${imgHeight + 300}px)`;
        img.style.width = `${imgWidth}px`;
        img.style.height = "auto";
        img.style.opacity = "0.9";
      }, 200);

      // Fade out and remove after 2 seconds
      setTimeout(() => {
        img.style.opacity = "0";
        setTimeout(() => img.remove(), 1000);
      }, 4000);

      const scale = 0.4; // 0.4 = 40% size, adjust as needed
      const smallCanvas = document.createElement("canvas");
      smallCanvas.width = canvas.width * scale;
      smallCanvas.height = canvas.height * scale;
      const ctx = smallCanvas.getContext("2d");
      ctx.drawImage(canvas, 0, 0, smallCanvas.width, smallCanvas.height);

      smallCanvas.toBlob((blob) => {
        if (blob) {
          // Save as screenshot.png
          // const url = URL.createObjectURL(blob);
          // const a = document.createElement("a");
          // a.href = url;
          // a.download = "screenshot.png";
          // document.body.appendChild(a);
          // a.click();
          // document.body.removeChild(a);
          // URL.revokeObjectURL(url);

          // Read as DataURL and log base64
          const reader = new FileReader();
          reader.onload = (e) => {
            console.log("PNG DataURL:", e.target?.result);
            const sizeStr = formatFileSize(blob.size);
            this._uploadedFiles.push({
              id: ++this._fileIdCounter,
              name: `Screenshot #${++this._screenshotIdCounter}`,
              content: e.target?.result as string,
              type: "image-screenshot",
              size: sizeStr,
            });
            renderFileList.call(this);
          };
          reader.readAsDataURL(blob);
        }
      }, "image/png");
    };

    actionRow.appendChild(screenshotUploadBtn);
    this._screenshotUploadBtn = screenshotUploadBtn;

    chatContent.appendChild(actionRow);




    this._setTraceComponentNames();

    // Upload Daisen Trace button
    const uploadTraceBtn = document.createElement("button");
    uploadTraceBtn.type = "button";
    uploadTraceBtn.title = "Upload Daisen Trace";
    uploadTraceBtn.style.background = "#f6f8fa";
    uploadTraceBtn.style.border = "1px solid #ccc";
    uploadTraceBtn.style.borderRadius = "6px";
    uploadTraceBtn.style.width = "38px";
    uploadTraceBtn.style.height = "38px";
    uploadTraceBtn.style.display = "flex";
    uploadTraceBtn.style.alignItems = "center";
    uploadTraceBtn.style.justifyContent = "center";
    uploadTraceBtn.style.cursor = "pointer";
    uploadTraceBtn.style.marginLeft = "4px";
    uploadTraceBtn.innerHTML = `
      <svg width="24" height="24" viewBox="0 0 20 20" fill="currentColor">
        <path d="m 15.3352 18.5003 v -1.8349 h -1.835 c -0.3671 0 -0.6648 -0.298 -0.665 -0.6651 c 0 -0.3672 0.2978 -0.665 0.665 -0.665 h 1.835 v -1.835 c 0 -0.3672 0.2978 -0.665 0.665 -0.665 c 0.3672 0.0002 0.6651 0.2979 0.6651 0.665 v 1.835 h 1.8349 l 0.1338 0.0137 c 0.303 0.062 0.5313 0.33 0.5313 0.6513 c -0.0002 0.3212 -0.2284 0.5894 -0.5313 0.6514 l -0.1338 0.0137 h -1.8349 v 1.8349 c -0.0002 0.367 -0.298 0.6649 -0.6651 0.6651 c -0.3671 0 -0.6648 -0.298 -0.665 -0.6651 z M 9.9455 1.0605 C 5.1695 1.0564 1.569 2.4872 1.569 4.3787 l 0 11.3369 c 0 1.8915 3.6004 3.3183 8.3765 3.3183 c 0.4553 0 0.9106 -0.0121 1.3519 -0.0404 c 0.3113 -0.0202 0.5482 -0.2547 0.5296 -0.5296 c -0.0232 -0.2708 -0.2927 -0.4728 -0.6085 -0.4607 c -0.4181 0.0242 -0.8456 0.0404 -1.2729 0.0404 c -4.2649 0 -7.2383 -1.2287 -7.2383 -2.3281 l 0 -2.0612 c 1.4263 0.974 4.0745 1.6006 7.2383 1.6006 c 0.2881 0 0.5807 -0.0041 0.8641 -0.0162 c 0.316 -0.0121 0.5575 -0.2425 0.5436 -0.5174 c -0.0139 -0.2749 -0.2787 -0.485 -0.5947 -0.4729 c -0.2694 0.0121 -0.5436 0.0162 -0.813 0.0162 c -4.2649 0 -7.2383 -1.2287 -7.2383 -2.3281 l 0 -2.0572 c 1.4263 0.974 4.0745 1.6005 7.2383 1.6005 c 4.776 0 8.3766 -1.4267 8.3766 -3.3182 l 0 -3.7831 c 0 -1.8915 -3.6007 -3.3182 -8.3766 -3.3182 z m 0 0.9862 c 4.2649 0 7.243 1.2286 7.243 2.332 c 0 1.1034 -2.9734 2.3281 -7.2384 2.3281 c -4.2649 0 -7.243 -1.2287 -7.243 -2.3281 c 0 -1.0993 2.9734 -2.332 7.2383 -2.332 z M 2.7072 6.0964 c 1.4263 0.974 4.0745 1.6006 7.2383 1.6006 c 3.1639 0 5.8121 -0.6265 7.2384 -1.6006 l 0 2.0612 c 0.0046 1.1034 -2.9688 2.3281 -7.2384 2.3281 c -4.2649 0 -7.2383 -1.2287 -7.2383 -2.3281 l 0 -2.0612 Z"/>
      </svg>
    `;
    /////////////////////////////////////

    // Create the floating div (hidden by default)
    const uploadTraceDiv = document.createElement("div");
    uploadTraceDiv.style.position = "absolute";
    uploadTraceDiv.style.left = "50px";
    uploadTraceDiv.style.bottom = "44px"; // 38px button + 4px gap
    uploadTraceDiv.style.background = "#fff";
    uploadTraceDiv.style.border = "1px solid #ccc";
    uploadTraceDiv.style.borderRadius = "8px";
    uploadTraceDiv.style.boxShadow = "0 2px 8px rgba(0,0,0,0.08)";
    uploadTraceDiv.style.padding = "12px 16px";
    uploadTraceDiv.style.zIndex = "10001";
    uploadTraceDiv.style.display = "none";
    uploadTraceDiv.style.width = "400px";

    // Create the button row for period controls
    const uploadTracePeriodButtonRow = document.createElement("div");
    uploadTracePeriodButtonRow.style.display = "flex";
    uploadTracePeriodButtonRow.style.justifyContent = "flex-start";
    uploadTracePeriodButtonRow.style.alignItems = "center";
    uploadTracePeriodButtonRow.style.marginBottom = "8px";

    // Create the Reset Trace Period button
    const uploadTraceResetPeriodBtn = document.createElement("button");
    uploadTraceResetPeriodBtn.textContent = "Reset Trace Period";
    uploadTraceResetPeriodBtn.style.background = "#0d6efd";
    uploadTraceResetPeriodBtn.style.color = "#fff";
    uploadTraceResetPeriodBtn.style.border = "none";
    uploadTraceResetPeriodBtn.style.borderRadius = "4px";
    uploadTraceResetPeriodBtn.style.padding = "4px 10px";
    uploadTraceResetPeriodBtn.style.cursor = "pointer";
    uploadTraceResetPeriodBtn.style.fontSize = "13px";
    uploadTraceResetPeriodBtn.style.marginRight = "8px";

    // Reset logic
    uploadTraceResetPeriodBtn.onclick = () => {
      this._traceSelectedStartTime = this._traceStartTime;
      this._traceSelectedEndTime = this._traceEndTime;
      updateSticksAndEdits.call(this);
    };

    uploadTracePeriodButtonRow.appendChild(uploadTraceResetPeriodBtn);

    // Create the improved toggle switch button for unit
    const uploadTraceSwitchUnitBtn = document.createElement("button");
    uploadTraceSwitchUnitBtn.type = "button";
    uploadTraceSwitchUnitBtn.title = "Switch between μs and s";
    uploadTraceSwitchUnitBtn.style.background = "#0d6efd";
    uploadTraceSwitchUnitBtn.style.border = "none";
    uploadTraceSwitchUnitBtn.style.borderRadius = "4px";
    uploadTraceSwitchUnitBtn.style.padding = "0";
    uploadTraceSwitchUnitBtn.style.width = "56px";
    uploadTraceSwitchUnitBtn.style.height = "27.5px";
    uploadTraceSwitchUnitBtn.style.display = "flex";
    uploadTraceSwitchUnitBtn.style.alignItems = "center";
    uploadTraceSwitchUnitBtn.style.position = "relative";
    uploadTraceSwitchUnitBtn.style.cursor = "pointer";
    uploadTraceSwitchUnitBtn.style.transition = "background 0.2s";
    uploadTraceSwitchUnitBtn.style.marginRight = "8px";
    uploadTraceSwitchUnitBtn.style.boxShadow = "0 1px 4px rgba(0,0,0,0.10)";

    // Remove any textContent
    uploadTraceSwitchUnitBtn.textContent = "";

    // Add the inner rounded square (the "thumb")
    const switchThumb = document.createElement("div");
    switchThumb.style.position = "absolute";
    switchThumb.style.left = "6px";
    switchThumb.style.top = "5px";
    switchThumb.style.width = "17.5px";
    switchThumb.style.height = "17.5px";
    switchThumb.style.background = "#fff";
    switchThumb.style.borderRadius = "4px";
    // switchThumb.style.boxShadow = "0 2px 8px rgba(0,0,0,0.15)";
    // switchThumb.style.boxShadow = "inset 2px 0 6px -2px rgba(0,0,0,0.10), inset 0 2px 6px -2px rgba(0,0,0,0.13)";
    switchThumb.style.transition = "left 0.25s cubic-bezier(0.4, 0, 0.2, 1)";
    switchThumb.style.zIndex = "2";
    uploadTraceSwitchUnitBtn.appendChild(switchThumb);

    // Add the "μs" and "s" labels
    const usLabel = document.createElement("span");
    usLabel.textContent = "μs";
    usLabel.style.position = "absolute";
    usLabel.style.right = "10px";
    usLabel.style.color = "#fff";
    // usLabel.style.fontWeight = "bold";
    usLabel.style.fontSize = "13px";
    usLabel.style.zIndex = "1";
    usLabel.style.transition = "opacity 0.2s";
    uploadTraceSwitchUnitBtn.appendChild(usLabel);

    const sLabel = document.createElement("span");
    sLabel.textContent = "s";
    sLabel.style.position = "absolute";
    sLabel.style.left = "14px";
    sLabel.style.color = "#fff";
    // sLabel.style.fontWeight = "bold";
    sLabel.style.fontSize = "13px";
    sLabel.style.zIndex = "1";
    sLabel.style.transition = "opacity 0.2s";
    uploadTraceSwitchUnitBtn.appendChild(sLabel);

    // Helper to update the switch UI
    const updateSwitchUI = () => {
    if (this._tracePeriodUnitSwitch) {
        // μs mode: thumb left, μs visible, s faded
        switchThumb.style.left = "6px";
        usLabel.style.opacity = "1";
        sLabel.style.opacity = "0.0";
        uploadTraceSwitchUnitBtn.style.background = "#0d6efd";
    } else {
        // s mode: thumb right, s visible, μs faded
        switchThumb.style.left = "32.5px";
        usLabel.style.opacity = "0.0";
        sLabel.style.opacity = "1";
        uploadTraceSwitchUnitBtn.style.background = "#0d6efd";
    }
    };
    updateSwitchUI();

    // Toggle logic
    uploadTraceSwitchUnitBtn.onclick = () => {
      this._tracePeriodUnitSwitch = !this._tracePeriodUnitSwitch;
      updateSwitchUI();
      updateSticksAndEdits.call(this);
    };

    uploadTracePeriodButtonRow.appendChild(uploadTraceSwitchUnitBtn);

    uploadTraceDiv.appendChild(uploadTracePeriodButtonRow);

    // Upload Trace Period Div
    const uploadTracePeriodDiv = document.createElement("div");
    uploadTracePeriodDiv.style.display = "flex";
    uploadTracePeriodDiv.style.flexDirection = "column";
    uploadTracePeriodDiv.style.marginBottom = "12px";

    // --- Row 1: Trail with two movable sticks ---
    const trailRow = document.createElement("div");
    trailRow.style.position = "relative";
    trailRow.style.height = "30px";
    trailRow.style.marginTop = "8px";
    // trailRow.style.marginBottom = "8px";
    trailRow.style.display = "flex";
    trailRow.style.alignItems = "center";
    trailRow.style.justifyContent = "center";

    // Trail bar
    const trailBar = document.createElement("div");
    trailBar.style.position = "absolute";
    trailBar.style.left = "16px";
    trailBar.style.right = "16px";
    trailBar.style.top = "14px";
    trailBar.style.height = "2px";
    trailBar.style.background = "#eee";
    trailBar.style.borderRadius = "2px";
    trailRow.appendChild(trailBar);

    // Add blue selection bar (wider than trailBar)
    const selectionBar = document.createElement("div");
    selectionBar.style.position = "absolute";
    selectionBar.style.top = "13px"; // slightly above trailBar
    selectionBar.style.height = "4px"; // wider than trailBar
    selectionBar.style.background = "#0d6efd";
    selectionBar.style.borderRadius = "5px";
    selectionBar.style.zIndex = "1";
    trailRow.appendChild(selectionBar);

    // Stickers
    const stick1 = document.createElement("div");
    const stick2 = document.createElement("div");
    [stick1, stick2].forEach(stick => {
      stick.style.position = "absolute";
      stick.style.top = "5px";
      stick.style.width = "12px";
      stick.style.height = "20px";
      stick.style.background = "#0d6efd";
      stick.style.border = "1px solid rgb(255, 255, 255)";
      stick.style.borderRadius = "4px";
      stick.style.cursor = "pointer";
      stick.style.boxShadow = "0 1px 4px rgba(0,0,0,0.10)";
      stick.style.zIndex = "2";
    });

    // --- Row 2: Text edits for start/end time ---
    const textRow = document.createElement("div");
    textRow.style.display = "flex";
    textRow.style.justifyContent = "space-between";
    textRow.style.alignItems = "center";
    textRow.style.marginTop = "2px";

    const startTimeEdit = document.createElement("input");
    startTimeEdit.type = "number";
    startTimeEdit.style.width = "110px";
    startTimeEdit.style.border = "1px solid #ccc";
    startTimeEdit.style.fontSize = "15px";
    startTimeEdit.style.height = "32px";
    startTimeEdit.style.boxSizing = "border-box";
    startTimeEdit.style.borderRadius = "4px";
    startTimeEdit.style.padding = "2px 6px";
    startTimeEdit.style.marginRight = "0px";
    startTimeEdit.style.overflow = "hidden";
    startTimeEdit.style.resize = "none";
    startTimeEdit.style.appearance = "textfield";

    const endTimeEdit = document.createElement("input");
    endTimeEdit.type = "number";
    endTimeEdit.style.width = "110px";
    endTimeEdit.style.fontSize = "15px";
    endTimeEdit.style.height = "32px";
    endTimeEdit.style.boxSizing = "border-box";
    endTimeEdit.style.border = "1px solid #ccc";
    endTimeEdit.style.borderRadius = "4px";
    endTimeEdit.style.padding = "2px 6px";
    endTimeEdit.style.marginLeft = "0px";
    endTimeEdit.style.overflow = "hidden";
    endTimeEdit.style.resize = "none";
    endTimeEdit.style.appearance = "textfield";

    // const startUnitLabel = document.createElement("span");
    // const endUnitLabel = document.createElement("span");
    // startUnitLabel.style.marginLeft = "4px";
    // startUnitLabel.style.fontSize = "15px";
    // startUnitLabel.style.color = "#888";
    // endUnitLabel.style.marginLeft = "4px";
    // endUnitLabel.style.fontSize = "15px";
    // endUnitLabel.style.color = "#888";
    
    // Helper for Greek mu (μ)
    const getUnitText = () => this._tracePeriodUnitSwitch ? "μs" : "s";

    // // Initial unit label
    // startUnitLabel.textContent = getUnitText();
    // endUnitLabel.textContent = getUnitText();

    // textRow.appendChild(startTimeEdit);
    // textRow.appendChild(startUnitLabel);
    // textRow.appendChild(endTimeEdit);
    // textRow.appendChild(endUnitLabel);

    const startTimeRow = document.createElement("div");
    startTimeRow.style.display = "flex";
    startTimeRow.style.alignItems = "center";
    startTimeRow.appendChild(startTimeEdit);
    // startUnitLabel.style.marginLeft = "2px"; // reduce gap
    // startTimeRow.appendChild(startUnitLabel);

    const endTimeRow = document.createElement("div");
    endTimeRow.style.display = "flex";
    endTimeRow.style.alignItems = "center";
    endTimeRow.appendChild(endTimeEdit);
    // endUnitLabel.style.marginLeft = "2px"; // reduce gap
    // endTimeRow.appendChild(endUnitLabel);

    textRow.appendChild(startTimeRow);
    textRow.appendChild(endTimeRow);

    // --- Logic for syncing sticks and edits ---
    this._traceSelectedStartTime = this._traceStartTime;
    this._traceSelectedEndTime = this._traceEndTime;

    // Helper: get trail bar pixel range
    const trailLeft = 16, trailRight = 344; // 400px maxWidth - 2*16px - 2*12px
    const trailWidth = trailRight - trailLeft;

    // Helper: convert time to position
    const timeToPos = (time: number) => {
      return trailLeft + ((time - this._traceStartTime) / (this._traceEndTime - this._traceStartTime)) * trailWidth;
    };
    const posToTime = (pos: number) => {
      return this._traceStartTime + ((pos - trailLeft) / trailWidth) * (this._traceEndTime - this._traceStartTime);
    };

    // Initial positions
    const updateSticksAndEdits = () => {
      // Decide which stick is start/end
      let s = Math.min(this._traceSelectedStartTime, this._traceSelectedEndTime);
      let e = Math.max(this._traceSelectedStartTime, this._traceSelectedEndTime);
      const pos1 = timeToPos(this._traceSelectedStartTime) - 6;
      const pos2 = timeToPos(this._traceSelectedEndTime) - 6;
      stick1.style.left = pos1 + "px";
      stick2.style.left = pos2 + "px";
      // Display in us if switch is true, otherwise s
      if (this._tracePeriodUnitSwitch) {
        startTimeEdit.value = (s * 1e6).toFixed(3);
        endTimeEdit.value = (e * 1e6).toFixed(3);
        startTimeEdit.placeholder = "Start (μs)";
        endTimeEdit.placeholder = "End (μs)";
        // startUnitLabel.textContent = "μs";
        // endUnitLabel.textContent = "μs";
      } else {
        startTimeEdit.value = s.toFixed(9);
        endTimeEdit.value = e.toFixed(9);
        startTimeEdit.placeholder = "Start (s)";
        endTimeEdit.placeholder = "End (s)";
        // startUnitLabel.textContent = "s";
        // endUnitLabel.textContent = "s";
      }

      // Draw blue selection bar between sticks
      const left = Math.min(pos1, pos2);
      const right = Math.max(pos1, pos2) + 12; // 12px is stick width
      selectionBar.style.left = left + "px";
      selectionBar.style.width = (right - left) + "px";
    };
    updateSticksAndEdits();

    // Drag logic
    function makeStickDraggable(stick: HTMLDivElement, isStart: boolean) {
    let dragging = false;
    let offsetX = 0;
    stick.onmousedown = (e) => {
      dragging = true;
      offsetX = e.clientX - stick.getBoundingClientRect().left;
      document.body.style.userSelect = "none";
    };
    document.addEventListener("mousemove", (e) => {
      if (!dragging) return;
      let x = e.clientX - uploadTraceDiv.getBoundingClientRect().left - offsetX;
      x = Math.max(trailLeft - 6, Math.min(trailRight - 6, x));
      let time = posToTime.call(this, x + 6);
      if (isStart) {
        this._traceSelectedStartTime = Math.max(this._traceStartTime, Math.min(time, this._traceEndTime));
        // Clamp if crossing
        if (this._traceSelectedStartTime > this._traceSelectedEndTime) [this._traceSelectedStartTime, this._traceSelectedEndTime] = [this._traceSelectedEndTime, this._traceSelectedStartTime];
      } else {
        this._traceSelectedEndTime = Math.max(this._traceStartTime, Math.min(time, this._traceEndTime));
        if (this._traceSelectedEndTime < this._traceSelectedStartTime) [this._traceSelectedStartTime, this._traceSelectedEndTime] = [this._traceSelectedEndTime, this._traceSelectedStartTime];
      }
      updateSticksAndEdits.call(this);
    });
    document.addEventListener("mouseup", () => {
      dragging = false;
      document.body.style.userSelect = "";
    });
    }
    makeStickDraggable.call(this, stick1, true);
    makeStickDraggable.call(this, stick2, false);

    // Edit logic
    startTimeEdit.onchange = () => {
      let val = parseFloat(startTimeEdit.value);
      if (this._tracePeriodUnitSwitch) val /= 1e6;
      let e = Math.max(this._traceSelectedStartTime, this._traceSelectedEndTime);
      if (isNaN(val) || val < this._traceStartTime) val = this._traceStartTime;
      if (val > e) val = e;
      this._traceSelectedStartTime = val;
      updateSticksAndEdits.call(this);
    };
      endTimeEdit.onchange = () => {
      let val = parseFloat(endTimeEdit.value);
      if (this._tracePeriodUnitSwitch) val /= 1e6;
      let s = Math.min(this._traceSelectedStartTime, this._traceSelectedEndTime);
      if (isNaN(val) || val < s) val = s;
      if (val > this._traceEndTime) val = this._traceEndTime;
      this._traceSelectedEndTime = val;
      updateSticksAndEdits.call(this);
    };

    // Add sticks and rows to period div
    trailRow.appendChild(stick1);
    trailRow.appendChild(stick2);
    uploadTracePeriodDiv.appendChild(textRow);
    uploadTracePeriodDiv.appendChild(trailRow);
    

    // Insert period div above uploadTraceComponentButtonRow
    // uploadTraceDiv.insertBefore(uploadTracePeriodDiv, uploadTraceDiv.firstChild);
    uploadTraceDiv.appendChild(uploadTracePeriodDiv);

    // Add gap + light grey line + gap between period div and component button row
    const dividerContainer = document.createElement("div");
    dividerContainer.style.display = "flex";
    dividerContainer.style.flexDirection = "column";
    dividerContainer.style.alignItems = "stretch";
    dividerContainer.style.margin = "12px 0"; // gap above and below

    const dividerGapTop = document.createElement("div");
    dividerGapTop.style.height = "4px";

    const dividerLine = document.createElement("div");
    dividerLine.style.height = "1px";
    dividerLine.style.background = "rgb(204, 204, 204)";
    dividerLine.style.width = "100%";

    const dividerGapBottom = document.createElement("div");
    dividerGapBottom.style.height = "8px";

    dividerContainer.appendChild(dividerGapTop);
    dividerContainer.appendChild(dividerLine);
    dividerContainer.appendChild(dividerGapBottom);

    uploadTraceDiv.appendChild(dividerContainer);


    const currentRows = this._traceAllComponentNames.filter(row => this._traceCurrentComponentNames.includes(row));
    const otherRows = this._traceAllComponentNames.filter(row => !this._traceCurrentComponentNames.includes(row));
    const uploadTraceRows = [...currentRows, ...otherRows];

    const uploadTraceComponentButtonRow = document.createElement("div");
    uploadTraceComponentButtonRow.style.display = "flex";
    uploadTraceComponentButtonRow.style.justifyContent = "flex-start";
    uploadTraceComponentButtonRow.style.alignItems = "center";
    uploadTraceComponentButtonRow.style.marginBottom = "8px";

    // Select Current button
    const uploadTraceSelectCurrentBtn = document.createElement("button");
    uploadTraceSelectCurrentBtn.style.marginRight = "8px";
    uploadTraceSelectCurrentBtn.textContent = `Select Current (${currentRows.length})`;
    uploadTraceSelectCurrentBtn.title = "Select Current";
    uploadTraceSelectCurrentBtn.style.background = "#e53935";
    uploadTraceSelectCurrentBtn.style.color = "#fff";
    uploadTraceSelectCurrentBtn.style.border = "none";
    uploadTraceSelectCurrentBtn.style.borderRadius = "4px";
    uploadTraceSelectCurrentBtn.style.padding = "4px 10px";
    uploadTraceSelectCurrentBtn.style.cursor = "pointer";
    uploadTraceSelectCurrentBtn.style.fontSize = "13px";
    

    const uploadTraceSelectAllBtn = document.createElement("button");
    uploadTraceSelectAllBtn.style.marginRight = "8px";
    uploadTraceSelectAllBtn.textContent = "Select All";
    uploadTraceSelectAllBtn.style.background = "#0d6efd";
    uploadTraceSelectAllBtn.style.color = "#fff";
    uploadTraceSelectAllBtn.style.border = "none";
    uploadTraceSelectAllBtn.style.borderRadius = "4px";
    uploadTraceSelectAllBtn.style.padding = "4px 10px";
    uploadTraceSelectAllBtn.style.cursor = "pointer";
    uploadTraceSelectAllBtn.style.fontSize = "13px";

    const uploadTraceDeselectAllBtn = document.createElement("button");
    uploadTraceDeselectAllBtn.textContent = "Deselect All";
    uploadTraceDeselectAllBtn.style.background = "#6c757d";
    uploadTraceDeselectAllBtn.style.color = "#fff";
    uploadTraceDeselectAllBtn.style.border = "none";
    uploadTraceDeselectAllBtn.style.borderRadius = "4px";
    uploadTraceDeselectAllBtn.style.padding = "4px 10px";
    uploadTraceDeselectAllBtn.style.cursor = "pointer";
    uploadTraceDeselectAllBtn.style.fontSize = "13px";


    uploadTraceComponentButtonRow.appendChild(uploadTraceSelectCurrentBtn);
    uploadTraceComponentButtonRow.appendChild(uploadTraceSelectAllBtn);
    uploadTraceComponentButtonRow.appendChild(uploadTraceDeselectAllBtn);
    uploadTraceDiv.appendChild(uploadTraceComponentButtonRow);

    // Scrollable region for checkboxes
    const uploadTraceScrollRegion = document.createElement("div");
    uploadTraceScrollRegion.style.maxHeight = "300px";
    uploadTraceScrollRegion.style.overflowY = "auto";
    uploadTraceScrollRegion.style.paddingRight = "4px";

    // Store checkbox elements for easy access
    const UploadTraceCheckboxMap: { [key: string]: HTMLInputElement } = {};

    uploadTraceRows.forEach(row => {
      const rowDiv = document.createElement("div");
      rowDiv.style.display = "flex";
      rowDiv.style.alignItems = "center";
      rowDiv.style.justifyContent = "space-between";
      rowDiv.style.marginBottom = "8px";

      const label = document.createElement("span");
      label.textContent = row;
      label.style.fontSize = "15px";
      label.style.color = "#222";
      label.style.maxWidth = "300px";
      label.style.overflow = "hidden";
      label.style.textOverflow = "ellipsis";
      label.style.whiteSpace = "nowrap";

      // Highlight current rows
      if (currentRows.includes(row)) {
        label.style.color = "#e53935";
        label.style.fontWeight = "bold";
      } else {
        label.style.color = "#222";
        label.style.fontWeight = "normal";
      }

      const checkbox = document.createElement("input");
      checkbox.type = "checkbox";
      checkbox.checked = (row in this._uploadTraceChecks) ? this._uploadTraceChecks[row] : false;
      checkbox.onchange = () => {
        this._uploadTraceChecks[row] = checkbox.checked;
        const uploadTraceCheckedCount = Object.values(this._uploadTraceChecks).filter(Boolean).length;
        this._renderBubble(uploadTraceBtn, uploadTraceCheckedCount, "bubble-upload-trace");
        // Update color after change
        if (currentRows.includes(row) && checkbox.checked) {
          checkbox.style.accentColor = "#e53935";
        } else {
          checkbox.style.accentColor = "";
        }
      };
      checkbox.style.marginLeft = "8px";
      UploadTraceCheckboxMap[row] = checkbox;

      // Make clicking the label toggle the checkbox
      label.onclick = () => {
        checkbox.checked = !checkbox.checked;
        checkbox.dispatchEvent(new Event("change"));
      };

      rowDiv.appendChild(label);
      rowDiv.appendChild(checkbox);
      uploadTraceScrollRegion.appendChild(rowDiv);
    });

    uploadTraceDiv.appendChild(uploadTraceScrollRegion);

    // Select Current logic
    uploadTraceSelectCurrentBtn.onclick = () => {
      currentRows.forEach(row => {
        this._uploadTraceChecks[row] = true;
        if (UploadTraceCheckboxMap[row]) {
          UploadTraceCheckboxMap[row].checked = true;
          UploadTraceCheckboxMap[row].style.accentColor = "#e53935";
        }
      });
      const uploadTraceCheckedCount = Object.values(this._uploadTraceChecks).filter(Boolean).length;
      this._renderBubble(uploadTraceBtn, uploadTraceCheckedCount, "bubble-upload-trace");
    };

    // Select All / Deselect All logic
    uploadTraceSelectAllBtn.onclick = () => {
      uploadTraceRows.forEach(row => {
        this._uploadTraceChecks[row] = true;
        UploadTraceCheckboxMap[row].checked = true;
        if (currentRows.includes(row)) {
          UploadTraceCheckboxMap[row].style.accentColor = "#e53935";
        } else {
          UploadTraceCheckboxMap[row].style.accentColor = "";
        }
      });
      const uploadTraceCheckedCount = uploadTraceRows.length;
      this._renderBubble(uploadTraceBtn, uploadTraceCheckedCount, "bubble-upload-trace");
    };

    uploadTraceDeselectAllBtn.onclick = () => {
      uploadTraceRows.forEach(row => {
        this._uploadTraceChecks[row] = false;
        UploadTraceCheckboxMap[row].checked = false;
      });
      this._renderBubble(uploadTraceBtn, 0, "bubble-upload-trace");
    };


    // Insert uploadTraceDiv into actionRow (relative positioning)
    actionRow.style.position = "relative";
    actionRow.appendChild(uploadTraceDiv);

    // Toggle logic
    uploadTraceBtn.onclick = () => {
      this._setTraceComponentNames();

      // Prepare rows: current first, then others
      const currentRows = this._traceAllComponentNames.filter(row => this._traceCurrentComponentNames.includes(row));
      const otherRows = this._traceAllComponentNames.filter(row => !this._traceCurrentComponentNames.includes(row));
      const uploadTraceRows = [...currentRows, ...otherRows];

      uploadTraceSelectCurrentBtn.textContent = `Select Current (${currentRows.length})`;
      uploadTraceSelectCurrentBtn.onclick = () => {
        currentRows.forEach(row => {
          this._uploadTraceChecks[row] = true;
          if (UploadTraceCheckboxMap[row]) {
            UploadTraceCheckboxMap[row].checked = true;
            UploadTraceCheckboxMap[row].style.accentColor = "#e53935";
          }
        });
        const uploadTraceCheckedCount = Object.values(this._uploadTraceChecks).filter(Boolean).length;
        this._renderBubble(uploadTraceBtn, uploadTraceCheckedCount, "bubble-upload-trace");
      };

      // Optionally, clear and rebuild the uploadTraceScrollRegion
      uploadTraceScrollRegion.innerHTML = "";
      uploadTraceRows.forEach(row => {
        const rowDiv = document.createElement("div");
        rowDiv.style.display = "flex";
        rowDiv.style.alignItems = "center";
        rowDiv.style.justifyContent = "space-between";
        rowDiv.style.marginBottom = "8px";

        const label = document.createElement("span");
        label.textContent = row;
        label.style.fontSize = "15px";
        label.style.maxWidth = "300px";
        label.style.overflow = "hidden";
        label.style.textOverflow = "ellipsis";
        label.style.whiteSpace = "nowrap";
        label.style.cursor = "pointer";

        if (currentRows.includes(row)) {
          label.style.color = "#e53935";
          label.style.fontWeight = "bold";
        } else {
          label.style.color = "#222";
          label.style.fontWeight = "normal";
        }

        const checkbox = document.createElement("input");
        checkbox.type = "checkbox";
        checkbox.checked = (row in this._uploadTraceChecks) ? this._uploadTraceChecks[row] : false;
        checkbox.onchange = () => {
          this._uploadTraceChecks[row] = checkbox.checked;
          const uploadTraceCheckedCount = Object.values(this._uploadTraceChecks).filter(Boolean).length;
          this._renderBubble(uploadTraceBtn, uploadTraceCheckedCount, "bubble-upload-trace");
          // Update color after change
          if (currentRows.includes(row) && checkbox.checked) {
            checkbox.style.accentColor = "#e53935";
          } else {
            checkbox.style.accentColor = "";
          }
        };
        checkbox.style.marginLeft = "8px";
        // *** Add this block to set accent color on initial render ***
        if (currentRows.includes(row) && checkbox.checked) {
          checkbox.style.accentColor = "#e53935";
        } else {
          checkbox.style.accentColor = "";
        }
        UploadTraceCheckboxMap[row] = checkbox;

        // Make clicking the label toggle the checkbox
        label.onclick = () => {
          checkbox.checked = !checkbox.checked;
          checkbox.dispatchEvent(new Event("change"));
        };

        rowDiv.appendChild(label);
        rowDiv.appendChild(checkbox);
        uploadTraceScrollRegion.appendChild(rowDiv);
      });


      this._uploadTraceVisible = !this._uploadTraceVisible;
      if (this._uploadTraceVisible) {
        uploadTraceDiv.style.display = "block";
        uploadTraceBtn.style.background = "#0d6efd";
        uploadTraceBtn.style.color = "#fff";
        if (this._attachRepoVisible) {
          this._attachRepoVisible = false; // Hide attach repo if trace upload is shown
          attachRepoDiv.style.display = "none";
          attachRepoBtn.style.background = "#f6f8fa";
          attachRepoBtn.style.color = "#222";
        }
      } else {
        uploadTraceDiv.style.display = "none";
        uploadTraceBtn.style.background = "#f6f8fa";
        uploadTraceBtn.style.color = "#222";
      }
    };
    















    /////////////////////////////////////


    actionRow.appendChild(uploadTraceBtn);

    // Attach Repository Code button
    const attachRepoBtn = document.createElement("button");
    attachRepoBtn.type = "button";
    attachRepoBtn.title = "Attach Repository Code";
    attachRepoBtn.style.background = "#f6f8fa";
    attachRepoBtn.style.border = "1px solid #ccc";
    attachRepoBtn.style.borderRadius = "6px";
    attachRepoBtn.style.width = "38px";
    attachRepoBtn.style.height = "38px";
    attachRepoBtn.style.display = "flex";
    attachRepoBtn.style.alignItems = "center";
    attachRepoBtn.style.justifyContent = "center";
    attachRepoBtn.style.cursor = "pointer";
    attachRepoBtn.style.marginLeft = "4px";
    attachRepoBtn.innerHTML = `
      <svg width="24" height="24" viewBox="0 -1 20 20" fill="currentColor">
        <path d="m 12.3352 17.5003 v -1.8349 h -1.835 c -0.3671 0 -0.6648 -0.298 -0.665 -0.6651 c 0 -0.3672 0.2978 -0.665 0.665 -0.665 h 1.835 v -1.835 c 0 -0.3672 0.2978 -0.665 0.665 -0.665 c 0.3672 0.0002 0.6651 0.2979 0.6651 0.665 v 1.835 h 1.8349 l 0.1338 0.0137 c 0.303 0.062 0.5313 0.33 0.5313 0.6513 c -0.0002 0.3212 -0.2284 0.5894 -0.5313 0.6514 l -0.1338 0.0137 h -1.8349 v 1.8349 c -0.0002 0.367 -0.298 0.6649 -0.6651 0.6651 c -0.3671 0 -0.6648 -0.298 -0.665 -0.6651 z M 5.5975 5.7671 L 2.4134 8.56 l 3.184 2.793 a 0.66 0.627 90 1 1 -0.8027 1.0141 l -3.762 -3.3 a 0.66 0.627 90 0 1 0 -1.0141 l 3.762 -3.3 a 0.66 0.627 90 1 1 0.8027 1.0141 Z m 13.794 2.286 l -3.762 -3.3 a 0.66 0.627 90 1 0 -0.8027 1.0141 L 18.0105 8.56 l -3.184 2.793 a 0.66 0.627 90 1 0 0.8027 1.0141 l 3.762 -3.3 a 0.66 0.627 90 0 0 0 -1.0141 Z m -6.4571 -7.3734 a 0.6603 0.6273 90 0 0 -0.8035 0.3947 l -5.016 14.52 a 0.66 0.627 90 1 0 1.1787 0.4513 l 5.016 -14.52 A 0.6602 0.6272 90 0 0 12.9342 0.6796 Z"  />
      </svg>
    `;
    // Create the floating div (hidden by default)
    const attachRepoDiv = document.createElement("div");
    attachRepoDiv.style.position = "absolute";
    attachRepoDiv.style.left = "100px";
    attachRepoDiv.style.bottom = "44px"; // 38px button + 4px gap
    attachRepoDiv.style.background = "#fff";
    attachRepoDiv.style.border = "1px solid #ccc";
    attachRepoDiv.style.borderRadius = "8px";
    attachRepoDiv.style.boxShadow = "0 2px 8px rgba(0,0,0,0.08)";
    attachRepoDiv.style.padding = "12px 16px";
    attachRepoDiv.style.zIndex = "10001";
    attachRepoDiv.style.display = "none";
    attachRepoDiv.style.maxWidth = "340px";

    // Add rows
    let attachRepoRows = [];
    if (this._githubIsAvailableResponse && this._githubIsAvailableResponse.available === 1) {
      attachRepoRows = [...this._githubIsAvailableResponse.routine_keys].sort();
    }
    else {
      attachRepoBtn.disabled = true;
      attachRepoBtn.style.cursor = "default";
    }

    const attachRepoTopRow = document.createElement("div");
    attachRepoTopRow.style.display = "flex";
    attachRepoTopRow.style.justifyContent = "flex-start";
    attachRepoTopRow.style.alignItems = "center";
    attachRepoTopRow.style.marginBottom = "8px";

    const AttachRepoSelectAllBtn = document.createElement("button");
    AttachRepoSelectAllBtn.style.marginRight = "8px";
    AttachRepoSelectAllBtn.textContent = "Select All";
    AttachRepoSelectAllBtn.style.background = "#0d6efd";
    AttachRepoSelectAllBtn.style.color = "#fff";
    AttachRepoSelectAllBtn.style.border = "none";
    AttachRepoSelectAllBtn.style.borderRadius = "4px";
    AttachRepoSelectAllBtn.style.padding = "4px 10px";
    AttachRepoSelectAllBtn.style.cursor = "pointer";
    AttachRepoSelectAllBtn.style.fontSize = "13px";

    const AttachRepoDeselectAllBtn = document.createElement("button");
    AttachRepoDeselectAllBtn.textContent = "Deselect All";
    AttachRepoDeselectAllBtn.style.background = "#6c757d";
    AttachRepoDeselectAllBtn.style.color = "#fff";
    AttachRepoDeselectAllBtn.style.border = "none";
    AttachRepoDeselectAllBtn.style.borderRadius = "4px";
    AttachRepoDeselectAllBtn.style.padding = "4px 10px";
    AttachRepoDeselectAllBtn.style.cursor = "pointer";
    AttachRepoDeselectAllBtn.style.fontSize = "13px";

    attachRepoTopRow.appendChild(AttachRepoSelectAllBtn);
    attachRepoTopRow.appendChild(AttachRepoDeselectAllBtn);
    attachRepoDiv.appendChild(attachRepoTopRow);

    // Scrollable region for checkboxes
    const attachRepoScrollRegion = document.createElement("div");
    attachRepoScrollRegion.style.maxHeight = "300px";
    attachRepoScrollRegion.style.overflowY = "auto";
    attachRepoScrollRegion.style.paddingRight = "4px";

    // Store checkbox elements for easy access
    const AttachRepoCheckboxMap: { [key: string]: HTMLInputElement } = {};

    attachRepoRows.forEach(row => {
      const rowDiv = document.createElement("div");
      rowDiv.style.display = "flex";
      rowDiv.style.alignItems = "center";
      rowDiv.style.justifyContent = "space-between";
      rowDiv.style.marginBottom = "8px";

      const label = document.createElement("span");
      label.textContent = row;
      label.style.fontSize = "15px";
      label.style.color = "#222";
      label.style.maxWidth = "280px";
      label.style.overflow = "hidden";
      label.style.textOverflow = "ellipsis";
      label.style.whiteSpace = "nowrap";
      label.style.cursor = "pointer";

      const checkbox = document.createElement("input");
      checkbox.type = "checkbox";
      checkbox.checked = (row in this._attachRepoChecks) ? this._attachRepoChecks[row] : false;
      checkbox.onchange = () => {
        this._attachRepoChecks[row] = checkbox.checked;
        const attachRepoCheckedCount = Object.values(this._attachRepoChecks).filter(Boolean).length;
        this._renderBubble(attachRepoBtn, attachRepoCheckedCount, "bubble-attach-repo");
      };
      checkbox.style.marginLeft = "8px";
      AttachRepoCheckboxMap[row] = checkbox;

      // Make clicking the label toggle the checkbox
      label.onclick = () => {
        checkbox.checked = !checkbox.checked;
        checkbox.dispatchEvent(new Event("change"));
      };

      rowDiv.appendChild(label);
      rowDiv.appendChild(checkbox);
      attachRepoScrollRegion.appendChild(rowDiv);
    });

    attachRepoDiv.appendChild(attachRepoScrollRegion);

    // Select All / Deselect All logic
    AttachRepoSelectAllBtn.onclick = () => {
      attachRepoRows.forEach(row => {
        this._attachRepoChecks[row] = true;
        AttachRepoCheckboxMap[row].checked = true;
      });
      const attachRepoCheckedCount = attachRepoRows.length;
      this._renderBubble(attachRepoBtn, attachRepoCheckedCount, "bubble-attach-repo");
    };

    AttachRepoDeselectAllBtn.onclick = () => {
      attachRepoRows.forEach(row => {
        this._attachRepoChecks[row] = false;
        AttachRepoCheckboxMap[row].checked = false;
      });
      this._renderBubble(attachRepoBtn, 0, "bubble-attach-repo");
    };


    // Insert attachRepoDiv into actionRow (relative positioning)
    actionRow.style.position = "relative";
    actionRow.appendChild(attachRepoDiv);

    // Toggle logic
    attachRepoBtn.onclick = () => {
      this._attachRepoVisible = !this._attachRepoVisible;
      if (this._attachRepoVisible) {
        attachRepoDiv.style.display = "block";
        attachRepoBtn.style.background = "#0d6efd";
        attachRepoBtn.style.color = "#fff";
        if (this._uploadTraceVisible) {
          this._uploadTraceVisible = false; // Hide trace upload if attach repo is shown
          uploadTraceDiv.style.display = "none";
          uploadTraceBtn.style.background = "#f6f8fa";
          uploadTraceBtn.style.color = "#222";
        }
      } else {
        attachRepoDiv.style.display = "none";
        attachRepoBtn.style.background = "#f6f8fa";
        attachRepoBtn.style.color = "#222";
      }
    };

    if (this._githubIsAvailableResponse && this._githubIsAvailableResponse.available === 0) {
      attachRepoBtn.disabled = true;
      attachRepoBtn.title = "Attach Repository Code\n(GitHub REST API is not available now)";
      // Add a slash from top-left to bottom-right over the icon
      attachRepoBtn.innerHTML = `
        <svg width="24" height="24" viewBox="0 0 256 256" fill="currentColor" style="position:relative;">
          <path d="M69.12158,94.14551,28.49658,128l40.625,33.85449a7.99987,7.99987,0,1,1-10.24316,12.291l-48-40a7.99963,7.99963,0,0,1,0-12.291l48-40a7.99987,7.99987,0,1,1,10.24316,12.291Zm176,27.709-48-40a7.99987,7.99987,0,1,0-10.24316,12.291L227.50342,128l-40.625,33.85449a7.99987,7.99987,0,1,0,10.24316,12.291l48-40a7.99963,7.99963,0,0,0,0-12.291Zm-82.38769-89.3734a8.00439,8.00439,0,0,0-10.25244,4.78418l-64,176a8.00034,8.00034,0,1,0,15.0371,5.46875l64-176A8.0008,8.0008,0,0,0,162.73389,32.48145Z"/>
          <line x1="6" y1="6" x2="18" y2="18" stroke="#e53935" stroke-width="2"/>
        </svg>
      `;
    }

    actionRow.appendChild(attachRepoBtn);

    // Graphtest button
    const graphTestBtn = document.createElement("button");
    graphTestBtn.type = "button";
    graphTestBtn.title = "Graph Test";
    graphTestBtn.style.background = "#f6f8fa";
    graphTestBtn.style.border = "1px solid #ccc";
    graphTestBtn.style.borderRadius = "6px";
    graphTestBtn.style.width = "38px";
    graphTestBtn.style.height = "38px";
    graphTestBtn.style.display = "flex";
    graphTestBtn.style.alignItems = "center";
    graphTestBtn.style.justifyContent = "center";
    graphTestBtn.style.cursor = "pointer";
    graphTestBtn.style.marginLeft = "4px";
    graphTestBtn.style.fontSize = "8px";
    graphTestBtn.style.fontWeight = "bold";
    graphTestBtn.style.fontFamily = "Arial, sans-serif";
    graphTestBtn.style.color = "#222";
    graphTestBtn.textContent = "Graph\nTest";

    graphTestBtn.style.opacity = "0";
    graphTestBtn.style.pointerEvents = "none";
    graphTestBtn.style.transition = "opacity 0.2s";
    
    // Add event listener for graphTest button
    let graphTestPrompt = "Please read the current trace file and count the number of each component class's event for me. Note that in the trace table the column \"Location\" may look like \"GPU[1].SA[1].CU[1].VALU\", you need to remove the \"[xx]\" part(s) in it to get the component class name, e.g. \"GPU.SA.CU.VALU\".";
    

    graphTestBtn.addEventListener("click", () => {
      this._graphTestButtonClicked = true; // Set flag to indicate button was clicked
      input.value = graphTestPrompt
      sendMessage();
    });

    // Subpagetest button
    const subpageTestBtn = document.createElement("button");
    subpageTestBtn.type = "button";
    subpageTestBtn.title = "Subpage Test";
    subpageTestBtn.style.background = "#f6f8fa";
    subpageTestBtn.style.border = "1px solid #ccc";
    subpageTestBtn.style.borderRadius = "6px";
    subpageTestBtn.style.width = "38px";
    subpageTestBtn.style.height = "38px";
    subpageTestBtn.style.display = "flex";
    subpageTestBtn.style.alignItems = "center";
    subpageTestBtn.style.justifyContent = "center";
    subpageTestBtn.style.cursor = "pointer";
    subpageTestBtn.style.marginLeft = "4px";
    subpageTestBtn.style.fontSize = "8px";
    subpageTestBtn.style.fontWeight = "bold";
    subpageTestBtn.style.fontFamily = "Arial, sans-serif";
    subpageTestBtn.style.color = "#222";
    subpageTestBtn.textContent = "Subpage\nTest";

    subpageTestBtn.style.opacity = "0";
    subpageTestBtn.style.pointerEvents = "none";
    subpageTestBtn.style.transition = "opacity 0.2s";

    // Add event listener for subpageTest button
    let subpageTestPrompt = "In this GPU task simulation, which takes longer—the host-to-GPU memory transfer or the kernel execution time? Also, what's the measured duration of the host-to-GPU memory copy?";

    subpageTestBtn.addEventListener("click", () => {
      this._subpageTestButtonClicked = true; // Set flag to indicate button was clicked
      input.value = subpageTestPrompt
      sendMessage();
    });


    actionRow.appendChild(graphTestBtn);
    actionRow.appendChild(subpageTestBtn);


    // Make test buttons appear on hover over
    // actionRow.addEventListener("mouseenter", () => {
    //   graphTestBtn.style.opacity = "1";
    //   graphTestBtn.style.pointerEvents = "auto";
    //   subpageTestBtn.style.opacity = "1";
    //   subpageTestBtn.style.pointerEvents = "auto";
    // });
    // actionRow.addEventListener("mouseleave", () => {
    //   graphTestBtn.style.opacity = "0";
    //   graphTestBtn.style.pointerEvents = "none";
    //   subpageTestBtn.style.opacity = "0";
    //   subpageTestBtn.style.pointerEvents = "none";
    // });


    // Initial bubble for file upload button
    this._renderBubble(fileUploadBtn, this._uploadedFiles.length, "bubble-upload-file");

    // Initial bubble for attach repo button
    const attachRepoCheckedCount = Object.values(this._attachRepoChecks).filter(Boolean).length;
    this._renderBubble(attachRepoBtn, attachRepoCheckedCount, "bubble-attach-repo");

    // Initial bubble for upload trace button
    const uploadTraceCheckedCount = Object.values(this._attachRepoChecks).filter(Boolean).length;
    this._renderBubble(attachRepoBtn, uploadTraceCheckedCount, "bubble-upload-trace");




    // Input area
    const inputContainer = document.createElement("div");
    inputContainer.style.display = "flex";
    inputContainer.style.gap = "8px";
    inputContainer.style.alignItems = "flex-end";

    const input = document.createElement("textarea");
    input.placeholder = "Ask anything (↑↓ for history)";
    input.rows = 1;
    input.style.flex = "1";
    input.style.padding = "6px";
    input.style.borderRadius = "4px";
    input.style.border = "1px solid #ccc";
    input.style.resize = "none";
    input.style.overflowY = "auto";
    input.style.minHeight = "38px";
    input.style.fontSize = "16px";
    input.style.maxHeight = "130px";

    // Auto-resize as user types
    input.addEventListener("input", function() {
      this.style.height = "auto";
      this.style.height = (this.scrollHeight) + "px";
    });

    // Add keyboard navigation for message history
    input.addEventListener("keydown", (e) => {
      // Check for arrow key navigation when input cursor is at start/end
      const atStart = input.selectionStart === 0 && input.selectionEnd === 0;
      const atEnd = input.selectionStart === input.value.length && input.selectionEnd === input.value.length;
      
      if (e.key === "ArrowUp" && (atStart || e.ctrlKey || e.metaKey)) {
        // Up arrow at start of input or Ctrl/Cmd + Up: Navigate to older messages
        e.preventDefault();
        this._navigateMessageHistory("up", input);
      } else if (e.key === "ArrowDown" && (atEnd || e.ctrlKey || e.metaKey)) {
        // Down arrow at end of input or Ctrl/Cmd + Down: Navigate to newer messages  
        e.preventDefault();
        this._navigateMessageHistory("down", input);
      } else if (e.key === "Enter" && !e.shiftKey) {
        // Send message on Enter (without Shift)
        e.preventDefault();
        sendMessage();
      } else if (e.key !== "ArrowUp" && e.key !== "ArrowDown") {
        // Reset navigation when user starts typing something new (except arrow keys for cursor movement)
        this._messageNavigationIndex = -1;
      }
    });

    const sendBtn = document.createElement("button");
    sendBtn.title = "Send";
    // sendBtn.className = "btn btn-primary";
    sendBtn.style.maxHeight = "38px";
    sendBtn.style.width = "38px";
    sendBtn.style.height = "38px";
    sendBtn.style.display = "flex";
    sendBtn.style.alignItems = "center";
    sendBtn.style.justifyContent = "center";
    sendBtn.style.borderRadius = "6px";
    sendBtn.style.background = "#0d6efd";
    sendBtn.style.border = "none";
    sendBtn.style.cursor = "pointer";
    // sendBtn.innerHTML = `
    //   <svg class="svg-icon" style="width: 24px; height: 24px; vertical-align: middle; fill: currentColor; overflow: hidden;" viewBox="0 0 1024 1024" xmlns="http://www.w3.org/2000/svg">
    //     <path d="M757.6 811.1 852.4 191 424.5 722.5c.6 67 .7 107.1.6 131.4L536 743c3.8-3.8 9.6-5 14.7-3.1l206.9 71.2ZM887.8 138.6c.1 1 .2 2 0 3.1L782.3 832.3c-.6 4.2-3.1 7.8-6.7 9.9-3.6 2-8 2.4-11.9.9L549.1 768.9 420.4 897.6c-2.7 2.7-6.2 4.1-9.8 4.1-2.5 0-5-.7-7.3-2.1-4.8-3-7.3-8.7-6.3-14.2.9-10.4.3-107.8-.2-167.4-.1-3 .9-6 2.8-8.5L817.2 191.2 174.6 562.9l171 102.5c6.5 4 8.4 12.5 4.4 19-2.6 4.2-7.1 6.5-11.7 6.5-2.5 0-5-.7-7.3-2.1L141.2 574.5c-4-2.5-6.5-6.9-6.5-11.7 0-4.7 2.4-9.2 6.5-11.7l725.3-423 .1-.1.2-.1c.2-.1.3-.1.5-.2 1.3-.8 2.7-1.3 4.2-1.6.4-.1.9-.1 1.3-.1 1.2-.1 2.4-.1 3.6.1.6.1 1.1.2 1.7.4.4.2.9.2 1.4.4.6.3 1 .7 1.5 1 .6.3 1.1.6 1.6 1 1 .7 1.9 1.7 2.6 2.7.2.2.5.4.7.7.1.1.1.2.1.3.9 1.4 1.5 3 1.8 4.7.1.4 0 .9 0 1.3Z" fill="#ffffff"/>
    //   </svg>
    // `;
    sendBtn.innerHTML = `
      <svg class="svg-icon" style="width: 20px; height: 20px; vertical-align: middle; fill: currentColor; overflow: hidden;" viewBox="0 0 386 386" xmlns="http://www.w3.org/2000/svg">
        <path d="M304.428 353.475l-101.2-87.2c-2-1.6-4-2.4-6.4-2-2.4 0-4.4 1.2-5.6 3.2l-50 72.4c-1.6 2.4-4 2-4.8 1.6-.8 0-2.8-1.2-2.8-3.6v-101.6c0-3.6-2.4-6.4-5.6-7.6l-112.4-37.6 351.2-153.2-62.4 315.6Zm0 16c1.6 0 3.6-.4 5.6-1.2 5.2-1.6 9.2-6.4 10.4-12l65.2-330.4c.8-2.8-.4-5.6-2.8-7.6s-5.6-2.4-8.4-1.2l-365.2 159.6c-5.6 2.8-9.6 8.8-9.2 15.2.4 6.8 4.4 12.4 10.8 14.8l106.8 35.6v96c0 8.8 5.6 16 14 18.8 8.8 2.8 17.6-.4 22.8-7.6l44.8-65.2 94.8 81.6c3.2 2.4 6.8 3.6 10.4 3.6Zm-106.8-89.2c2.4 0 4.8-1.2 6.4-3.2l176-240c2.8-3.6 2-8.4-1.6-11.2s-8.4-2-11.2 1.6l-176 240c-2.8 3.6-2 8.4 1.6 11.2 1.6 1.2 3.2 1.6 4.8 1.6Zm-46.4-57.2c1.6 0 3.6-.4 4.8-1.6l12.8-10.4c3.6-2.8 4-7.6 1.2-11.2s-7.6-4-11.2-1.2l-12.8 10.4c-3.6 2.8-4 7.6-1.2 11.2 1.6 1.6 4 2.8 6.4 2.8Zm34.4-28c1.6 0 3.6-.4 5.2-1.6l188-154.8c3.6-2.8 4-7.6 1.2-11.2-2.8-3.6-7.6-4-11.2-1.2l-188 154.8c-3.6 2.8-4 7.6-1.2 11.2 1.6 2 3.6 2.8 6 2.8Z" fill="#ffffff"/>
      </svg>
    `;

    // Send handler
    const sendMessage = async () => {
      const userMsg = input.value.trim();
      if (!userMsg) return;

      // Build uploaded files prefix if any
      let prefix = "";
      if (this._uploadedFiles.some(f => f.type === "file")) {
        prefix += "[Uploaded Files]\n";
        this._uploadedFiles.filter(f => f.type === "file").forEach(f => {
          prefix += `[Uploaded File "${f.name}"]\n${f.content}\n`;
        });
        prefix += "[End Uploaded Files]\n";
      }

      // Compose the full message
      const urlPrefix = `[Current URL (Remember it, but there is no need to mention it unless the user asks for it)] ${this._getCurrentFrontendURL()}\n`;
      // console.log("Current URL:", urlPrefix);
      let urlContent: any = [
        { type: "text", text: urlPrefix }
      ];
      messages.push({ role: "system", content: urlContent });
      const fullMsg = prefix + userMsg;
      // console.log("Full message to send:", fullMsg);
      const imageFiles = this._uploadedFiles.filter(f => f.type === "image" || f.type === "image-screenshot");
      let fullContentForAPI: any = [
        { type: "text", text: fullMsg }
      ];
      imageFiles.forEach(img => {
        fullContentForAPI.push({
          type: "image_url",
          image_url: { url: img.content }
        });
      });

      // Disable send button while waiting
      sendBtn.disabled = true;
      input.disabled = true;

      // User message
      const userDiv = document.createElement("div");
      userDiv.style.display = "flex";
      userDiv.style.flexDirection = "column";
      userDiv.style.alignItems = "flex-end";
      userDiv.style.margin = "4px 0";

      const userBubble = document.createElement("span");
      userBubble.innerHTML = "<b>You:</b> " + userMsg;
      userBubble.style.background = "#0d6efd";
      userBubble.style.color = "white";
      userBubble.style.padding = "8px 12px";
      userBubble.style.borderRadius = "16px";
      userBubble.style.maxWidth = "90%";
      userBubble.style.display = "inline-block";
      userBubble.style.wordBreak = "break-word";
      userDiv.appendChild(userBubble);

      // Add URL underneath the user bubble
      const urlDiv = document.createElement("div");
      urlDiv.innerHTML = this._getCurrentFrontendURL();
      urlDiv.style.fontSize = "10px";
      urlDiv.style.color = "#999999";
      urlDiv.style.marginTop = "2px";
      urlDiv.style.textAlign = "right";
      urlDiv.style.maxWidth = "90%";
      urlDiv.style.wordBreak = "break-all";
      userDiv.appendChild(urlDiv);

      messagesDiv.appendChild(userDiv);

      // Check if this is a "Generate graph." message from the graphTest button
      if (this._graphTestButtonClicked && (userMsg.toLowerCase() === graphTestPrompt.toLowerCase() || userMsg.toLowerCase() === "generate graph")) {
        // Reset the flag
        this._graphTestButtonClicked = false;
        
        // Add user message to chat history
        messages.push({ role: "user", content: [{ type: "text", text: fullMsg }] });
        this._chatMessages = messages;
        this._saveChatToHistory();
        
        // Clear input field
        input.value = "";
        input.style.height = "38px";
        
        // Show "loading" message while fetching CSV
        const botDiv = document.createElement("div");
        botDiv.innerHTML = `<b>Daisen Bot:</b> Loading graph data...`;
        botDiv.style.textAlign = "left";
        botDiv.style.margin = "4px 0";
        messagesDiv.appendChild(botDiv);
        
        // Fetch CSV data and display as table
        try {
          const csvData = await this._fetchGraphTestCSV();
          const tableHTML = this._csvToHTMLTable(csvData);
          localStorage.setItem("visualization_data", JSON.stringify(csvData));
          const graphLink = '<a href="http://localhost:5173/datavisualization.html" target="_blank" style="color: #0d6efd; text-decoration: underline;">http://localhost:5173/datavisualization.html</a>';
          const responseContent =
            `Got it! I've processed the trace file and summarized the event counts for each unique component class. Here's the table:<br>${tableHTML}` +
            `I've also generated a graph visualization of the distribution, which you can access here: ${graphLink}.<br>` +
            `Would you like me to also provide a Python script so you can reproduce this analysis on your own?`;

          // Update bot response with table
          botDiv.innerHTML = `<b>Daisen Bot:</b> ${responseContent}`;
          
          // Add bot message to chat history
          messages.push({ role: "assistant", content: [{ type: "text", text: responseContent }] });
        } catch (error) {
          // Handle error case
          const errorMessage = `Error loading graph data: ${error.message}`;
          botDiv.innerHTML = `<b>Daisen Bot:</b> ${errorMessage}`;
          messages.push({ role: "assistant", content: [{ type: "text", text: errorMessage }] });
        }
        
        this._chatMessages = messages;
        this._saveChatToHistory();
        messagesDiv.scrollTop = messagesDiv.scrollHeight;
        
        // Re-enable controls
        sendBtn.disabled = false;
        input.disabled = false;
        input.focus();
        
        // Clear uploaded files and reset UI state
        this._uploadedFiles = [];
        this._fileIdCounter = 0;
        renderFileList.call(this);

        // Clear all checkboxes and hide attachRepoDiv
        Object.keys(this._attachRepoChecks).forEach(key => {
          this._attachRepoChecks[key] = false;
          if (AttachRepoCheckboxMap[key]) AttachRepoCheckboxMap[key].checked = false;
        });
        this._attachRepoVisible = false;
        attachRepoDiv.style.display = "none";
        attachRepoBtn.style.background = "#f6f8fa";
        attachRepoBtn.style.color = "#222";
        this._renderBubble(attachRepoBtn, 0, "bubble-attach-repo");

        // Reset all checkboxes in uploadTraceDiv
        Object.keys(this._uploadTraceChecks).forEach(key => {
          this._uploadTraceChecks[key] = false;
          if (UploadTraceCheckboxMap[key]) UploadTraceCheckboxMap[key].checked = false;
        });
        this._uploadTraceVisible = false;
        uploadTraceDiv.style.display = "none";
        uploadTraceBtn.style.background = "#f6f8fa";
        uploadTraceBtn.style.color = "#222";
        this._renderBubble(uploadTraceBtn, 0, "bubble-upload-trace");

        // Reset selected start/end time and update sticks/textedits
        this._traceSelectedStartTime = this._traceStartTime;
        this._traceSelectedEndTime = this._traceEndTime;
        if (typeof updateSticksAndEdits === "function") updateSticksAndEdits.call(this);
        
        return; // Exit early, don't call GPT API
      }
      else if (this._subpageTestButtonClicked && (userMsg.toLowerCase() === subpageTestPrompt.toLowerCase() || userMsg.toLowerCase() === "generate subpage")) {
        // Handle subpage test button click
        // Reset the flag
        this._subpageTestButtonClicked = false;

        // Add user message to chat history
        messages.push({ role: "user", content: [{ type: "text", text: fullMsg }] });
        this._chatMessages = messages;
        this._saveChatToHistory();
        
        // Clear input field
        input.value = "";
        input.style.height = "38px";
        
        // Show "loading" message while fetching CSV
        const botDiv = document.createElement("div");
        botDiv.innerHTML = `<b>Daisen Bot:</b> Loading ...`;

        botDiv.style.textAlign = "left";
        botDiv.style.margin = "4px 0";
        messagesDiv.appendChild(botDiv);
        

        const subLink = '<a href="http://localhost:5173/task?id=d240btg3fvio1hp2d3eg" target="_blank" style="color: #0d6efd; text-decoration: underline;">http://localhost:5173/task?id=d240btg3fvio1hp2d3eg</a>';
        const responseContent =
            `In MGPUSim, the host-to-GPU memory transfer is recorded as **"MemCopyH2D"**. ` +
            `From this simulation, the duration of MemCopyH2D is **76.68 µs** (0 → 76.68 µs), ` +
            `while the kernel execution takes **7.97 µs** (76.68 → 84.65 µs). ` +
            `Therefore, the host-to-GPU memory transfer is longer than the kernel execution time. ` +
            `For more details, you can check the Task View Page here:<br>${subLink}.<br>` +
            `Would you like me to also generate a breakdown chart for better visualization?`;

        // Update bot response with table
        // botDiv.innerHTML = `<b>Daisen Bot:</b> ${responseContent}`;
        botDiv.innerHTML =
          `<b>Daisen Bot:</b> <span style="color:#aaa;font-size:0.95em;">(1,923 tokens)</span> ` +
          convertMarkdownToHTML(autoWrapMath(responseContent));

        const userDivSecond = document.createElement("div");
        userDivSecond.style.display = "flex";
        userDivSecond.style.justifyContent = "flex-end";
        userDivSecond.style.margin = "4px 0";

        const userMsgSecond = "yes"; // Simulate user response

        const userBubbleSecond = document.createElement("span");
        userBubbleSecond.innerHTML = "<b>You:</b> " + userMsgSecond;
        userBubbleSecond.style.background = "#0d6efd";
        userBubbleSecond.style.color = "white";
        userBubbleSecond.style.padding = "8px 12px";
        userBubbleSecond.style.borderRadius = "16px";
        userBubbleSecond.style.maxWidth = "90%";
        userBubbleSecond.style.display = "inline-block";
        userBubbleSecond.style.wordBreak = "break-word";
        userDivSecond.appendChild(userBubbleSecond);

        messagesDiv.appendChild(userDivSecond);

        const csvDataSub = await this._csvStringToArray(`Operation,Start (µs),End (µs),Duration (µs)
Host-to-GPU MemCopy,0.00,76.68,76.68
Kernel Execution,76.68,84.65,7.97`);
        const tableHTML = this._csvToHTMLTable(csvDataSub);

        const responseContentSecond = 
          `Great! I've generated a breakdown chart comparing the host-to-GPU memory copy time and the kernel execution time. ` +
          `This visualization will help you see how much longer the memory transfer takes compared to the kernel. ` +
          `Here's the detailed timing table:<br>${tableHTML}`
        const botDivSecond = document.createElement("div");
        botDivSecond.innerHTML = `<b>Daisen Bot:</b> Loading ...`;

        botDivSecond.style.textAlign = "left";
        botDivSecond.style.margin = "4px 0";
        messagesDiv.appendChild(botDivSecond);

        botDivSecond.innerHTML =
          `<b>Daisen Bot:</b> <span style="color:#aaa;font-size:0.95em;">(2,341 tokens)</span> ` +
          convertMarkdownToHTML(autoWrapMath(responseContentSecond));
        
        // Add bot message to chat history
        messages.push({ role: "assistant", content: [{ type: "text", text: responseContent }] });
        messages.push({ role: "user", content: [{ type: "text", text: userMsgSecond }] });
        messages.push({ role: "assistant", content: [{ type: "text", text: responseContentSecond }] });


        

        // this._chatMessages = messages; // Update the instance messages
        // this._saveChatToHistory(); // Save the updated chat
        // messagesDiv.scrollTop = messagesDiv.scrollHeight;
        // console.log("[Received from GPT]", gptResponse);

        // // Apply KaTeX rendering for math in the new messages
        // botDiv.querySelectorAll('.math').forEach(el => {
        //   try {
        //     const tex = el.textContent || "";
        //     const displayMode = el.getAttribute("data-display") === "block";
        //     console.log("Rendering math:", tex, "Display mode:", displayMode);
        //     el.innerHTML = katex.renderToString(tex, { displayMode });
        //   } catch (e) {
        //     el.innerHTML = "<span style='color:red'>Invalid math</span>";
        //     console.log("KaTeX error:", e, "for tex:", el.textContent);
        //   }
        // });
   
        
        this._chatMessages = messages;
        this._saveChatToHistory();
        messagesDiv.scrollTop = messagesDiv.scrollHeight;
        
        // Re-enable controls
        sendBtn.disabled = false;
        input.disabled = false;
        input.focus();
        
        // Clear uploaded files and reset UI state
        this._uploadedFiles = [];
        this._fileIdCounter = 0;
        renderFileList.call(this);

        // Clear all checkboxes and hide attachRepoDiv
        Object.keys(this._attachRepoChecks).forEach(key => {
          this._attachRepoChecks[key] = false;
          if (AttachRepoCheckboxMap[key]) AttachRepoCheckboxMap[key].checked = false;
        });
        this._attachRepoVisible = false;
        attachRepoDiv.style.display = "none";
        attachRepoBtn.style.background = "#f6f8fa";
        attachRepoBtn.style.color = "#222";
        this._renderBubble(attachRepoBtn, 0, "bubble-attach-repo");

        // Reset all checkboxes in uploadTraceDiv
        Object.keys(this._uploadTraceChecks).forEach(key => {
          this._uploadTraceChecks[key] = false;
          if (UploadTraceCheckboxMap[key]) UploadTraceCheckboxMap[key].checked = false;
        });
        this._uploadTraceVisible = false;
        uploadTraceDiv.style.display = "none";
        uploadTraceBtn.style.background = "#f6f8fa";
        uploadTraceBtn.style.color = "#222";
        this._renderBubble(uploadTraceBtn, 0, "bubble-upload-trace");

        // Reset selected start/end time and update sticks/textedits
        this._traceSelectedStartTime = this._traceStartTime;
        this._traceSelectedEndTime = this._traceEndTime;
        if (typeof updateSticksAndEdits === "function") updateSticksAndEdits.call(this);
        
        return; // Exit early, don't call GPT API
      }

      // Call GPT with full history
      messages.push({ role: "user", content: fullContentForAPI });
      this._chatMessages = messages; // Update the instance messages
      this._saveChatToHistory(); // Save the updated chat
      console.log("[Sent to GPT]", fullMsg);

      // Clear input field
      input.value = "";
      input.style.height = "38px"; // Reset to one line
      // Show "thinking message"
      const botDiv = document.createElement("div");
      botDiv.innerHTML = `<b>Daisen Bot:</b> Thinking...&nbsp;&nbsp;<span id="thinking-spinner">|</span>`;
      botDiv.style.textAlign = "left";
      botDiv.style.margin = "4px 0";
      messagesDiv.appendChild(botDiv);

      let dotCount = 1;
      const maxDots = 3;
      let spinnerIndex = 0;
      const spinnerChars = ["|", "/", "-", "\\"];
      // const thinkingDots = botDiv.querySelector("#thinking-dots");
      const thinkingSpinner = botDiv.querySelector("#thinking-spinner");
      const dotsInterval = setInterval(() => {
        dotCount = (dotCount % maxDots) + 1;
        spinnerIndex = (spinnerIndex + 1) % spinnerChars.length;
        // if (thinkingDots) {
        //     const dots = ".".repeat(dotCount) + "&nbsp;".repeat(maxDots - dotCount);
        //     thinkingDots.innerHTML = dots;
        // }

        if (thinkingSpinner) {
            thinkingSpinner.textContent = spinnerChars[spinnerIndex];
        }
      }, 500);

      // Call GPT and update the message
      const selectedGitHubRoutineKeys = Object.keys(this._attachRepoChecks).filter(k => this._attachRepoChecks[k]);
      const selectedComponentNameList: string[] = [];
      uploadTraceRows.forEach(row => {
        if (UploadTraceCheckboxMap[row] && UploadTraceCheckboxMap[row].checked) {
          selectedComponentNameList.push(row);
        }
      });
      const traceInfo = {
        selected: selectedComponentNameList.length > 0 ? 1 : 0,
        startTime: this._traceSelectedStartTime,
        endTime: this._traceSelectedEndTime,
        selectedComponentNameList: selectedComponentNameList,
      };
      const gptRequest: GPTRequest = {
        messages: messages,
        traceInfo: traceInfo,
        selectedGitHubRoutineKeys: selectedGitHubRoutineKeys
      };
      console.log("GPTRequest:", gptRequest);
      
      sendPostGPT(gptRequest).then((gptResponse) => {
        const gptResponseContent = gptResponse.content;
        const gptResponseTotalTokens = gptResponse.totalTokens;
        console.log("[Received from GPT - Cost] Total tokens used:", gptResponseTotalTokens !== -1 ? gptResponseTotalTokens : "unknown");
        botDiv.innerHTML =
          `<b>Daisen Bot:</b> <span style="color:#aaa;font-size:0.95em;">(${
            gptResponseTotalTokens === -1 ? "no tokens" : gptResponseTotalTokens.toLocaleString() + " tokens"
          })</span> ` +
          convertMarkdownToHTML(autoWrapMath(gptResponseContent));

        messages.push({ role: "assistant", content: [{"type": "text", "text": gptResponseContent}] });
        this._chatMessages = messages; // Update the instance messages
        this._saveChatToHistory(); // Save the updated chat
        messagesDiv.scrollTop = messagesDiv.scrollHeight;
        console.log("[Received from GPT]", gptResponse);

        // Apply KaTeX rendering for math in the new messages
        botDiv.querySelectorAll('.math').forEach(el => {
          try {
            const tex = el.textContent || "";
            const displayMode = el.getAttribute("data-display") === "block";
            console.log("Rendering math:", tex, "Display mode:", displayMode);
            el.innerHTML = katex.renderToString(tex, { displayMode });
          } catch (e) {
            el.innerHTML = "<span style='color:red'>Invalid math</span>";
            console.log("KaTeX error:", e, "for tex:", el.textContent);
          }
        });
        
        // Re-enable send button
        sendBtn.disabled = false;
        input.disabled = false;
        input.focus();
      });
      this._chatMessages = messages; // Update chat messages in the class

      // Clear uploaded files and reset index
      this._uploadedFiles = [];
      this._fileIdCounter = 0;
      renderFileList.call(this);

      // Clear all checkboxes and hide attachRepoDiv
      Object.keys(this._attachRepoChecks).forEach(key => {
        this._attachRepoChecks[key] = false;
        if (AttachRepoCheckboxMap[key]) AttachRepoCheckboxMap[key].checked = false;
      });
      this._attachRepoVisible = false;
      attachRepoDiv.style.display = "none";
      attachRepoBtn.style.background = "#f6f8fa";
      attachRepoBtn.style.color = "#222";
      this._renderBubble(attachRepoBtn, 0, "bubble-attach-repo");

      // Reset all checkboxes in uploadTraceDiv
      Object.keys(this._uploadTraceChecks).forEach(key => {
        this._uploadTraceChecks[key] = false;
        if (UploadTraceCheckboxMap[key]) UploadTraceCheckboxMap[key].checked = false;
      });
      this._uploadTraceVisible = false;
      uploadTraceDiv.style.display = "none";
      uploadTraceBtn.style.background = "#f6f8fa";
      uploadTraceBtn.style.color = "#222";
      this._renderBubble(uploadTraceBtn, 0, "bubble-upload-trace");

      // Reset selected start/end time and update sticks/textedits
      this._traceSelectedStartTime = this._traceStartTime;
      this._traceSelectedEndTime = this._traceEndTime;
      if (typeof updateSticksAndEdits === "function") updateSticksAndEdits.call(this);
    }

    sendBtn.onclick = sendMessage;
    input.addEventListener("keydown", (e) => {
      if (e.key === "Enter" && !e.shiftKey) {
        e.preventDefault();
        sendMessage();
      }
    });

    // The New Chat button functionality is now handled in the topBar creation above

    inputContainer.appendChild(input);
    inputContainer.appendChild(sendBtn);
    chatContent.appendChild(inputContainer);

    // document.body.appendChild(chatPanel);

    // // Animate in
    // setTimeout(() => {
    //   chatPanel.classList.add('open');
    //   // Hide the chat button
    //   this._showChatButton = false;
    //   this._addPaginationControl();
    // }, 200);
  }

    // Add this method to Dashboard class for reusable bubble rendering
  _renderBubble(btn: HTMLButtonElement, value: number, bubbleId: string) {
    let bubble = btn.querySelector(`#${bubbleId}`) as HTMLDivElement;
    if (!bubble) {
      bubble = document.createElement("div");
      bubble.id = bubbleId;
      bubble.style.position = "absolute";
      bubble.style.top = "-7px";
      bubble.style.right = "-7px";
      bubble.style.minWidth = "18px";
      bubble.style.height = "18px";
      bubble.style.background = "#e53935";
      bubble.style.color = "#fff";
      bubble.style.borderRadius = "50%";
      bubble.style.display = "flex";
      bubble.style.alignItems = "center";
      bubble.style.justifyContent = "center";
      bubble.style.fontSize = "10px";
      bubble.style.fontWeight = "bold";
      bubble.style.boxShadow = "0 1px 4px rgba(0,0,0,0.15)";
      bubble.style.pointerEvents = "none";
      bubble.style.zIndex = "10";
      bubble.style.padding = "0 4px";
      bubble.style.userSelect = "none";
      bubble.style.transition = "opacity 0.2s";
      bubble.style.opacity = value > 0 ? "1" : "0";
      btn.style.position = "relative";
      btn.appendChild(bubble);
    }
    let displayValue = "";
    if (value === 0) {
      bubble.style.opacity = "0";
      bubble.textContent = "";
    } else if (value > 99) {
      bubble.style.opacity = "1";
      bubble.textContent = "99+";
    } else {
      bubble.style.opacity = "1";
      bubble.textContent = value.toString();
    }
  }

  _injectChatPanelCSS() {
    if (document.getElementById('chat-panel-anim-style')) return;
    const style = document.createElement('style');
    style.id = 'chat-panel-anim-style';
    style.innerHTML = `
      #chat-panel {
        transition: transform 0.3s cubic-bezier(.4,0,.2,1), opacity 0.3s cubic-bezier(.4,0,.2,1);
        transform: translateX(100%);
        opacity: 0;
      }
      #chat-panel.open {
        transform: translateX(0);
        opacity: 1;
      }
      #chat-panel.closing {
        transform: translateX(100%);
        opacity: 0;
      }
    `;
    document.head.appendChild(style);
  }

  _getChatPanelWidth(): number {
    const chatPanel = document.getElementById("chat-panel");
    if (chatPanel) {
      const computedStyle = window.getComputedStyle(chatPanel);
      const width = parseInt(computedStyle.width);
      this._chatPanelWidth = width;
      console.log(`Chat Panel Width updated: ${width}px`);
      return width;
    } else {
      this._chatPanelWidth = 0;
      console.log(`Chat Panel not found, width set to 0px`);
      return 0;
    }
  }

  _getCurrentFrontendURL(): string {
    return window.location.href;
  }
}


function convertMarkdownToHTML(text: string): string {
  // // Headings: ###, ##, #
  // text = text.replace(/^### (.+)$/gm, '<h3>$1</h3>');
  // text = text.replace(/^## (.+)$/gm, '<h2>$1</h2>');
  // text = text.replace(/^# (.+)$/gm, '<h1>$1</h1>');
  // // Horizontal rule: ---
  // text = text.replace(/^-{3,}$/gm, '<hr>');
  // // Bold: **text**
  // text = text.replace(/\*\*(.+?)\*\*/g, "<b>$1</b>");
  // // Italic: *text*
  // text = text.replace(/\*(.+?)\*/g, "<i>$1</i>");
  // // // Inline code: `code`
  // // text = text.replace(/`([^`]+)`/g, "<code>$1</code>");
  // // Math: \[ ... \] (block)
  // text = text.replace(/\\\[(.+?)\\\]/gs, '<span class="math" data-display="block">$1</span>');
  // // Math: \( ... \) (inline)
  // text = text.replace(/\\\((.+?)\\\)/gs, '<span class="math" data-display="inline">$1</span>');
  // // Line breaks
  // text = text.replace(/\n/g, "<br>");
  // return text;
  text = text.replace(/```html\n([\s\S]*?)```/g, (match, code) => {
    // Remove leading/trailing empty lines
    let trimmed = code.replace(/^\s*\n+/, '').replace(/\n+\s*$/, '').replace(/(<br>\s*){1,}/g, "<br>");
    trimmed = trimmed.replace(/(<\/h[1-6]>|<\/hr>|<\/p>|<\/table>|<\/ul>|<\/ol>|<\/pre>|<\/div>|<\/span>)(<br>)+/g, "$1");
    trimmed = trimmed.replace(/(<br>\s*)+(<table)/g, "$2");
    // Remove leading <br> at the very start
    trimmed = trimmed.replace(/^(<br>\s*)+/, "");
    return trimmed;
  });
  
  // Code blocks: ```lang\ncode\n```
  text = text.replace(/```(\w*)\n([\s\S]*?)```/g, (match, lang, code) => {
    // Remove leading/trailing empty lines (including multiple)
    const trimmed = code.replace(/^\s*\n+/, '').replace(/\n+\s*$/, '');
    // Escape HTML special chars in code
    const escaped = trimmed.replace(/</g, "&lt;").replace(/>/g, "&gt;");
    return `<pre class="code-block"><code${lang ? ` class="language-${lang}"` : ""}>${escaped}</code></pre>`;
  });

  // Inline code: `code`
  text = text.replace(/`([^`]+)`/g, (match, code) => {
    const escaped = code.replace(/</g, "&lt;").replace(/>/g, "&gt;");
    return `<code class="inline-code">${escaped}</code>`;
  });

  // Headings: ###, ##, #
  text = text.replace(/^### (.+)$/gm, (match, p1) => {
    console.log("Matched h3:", match);
    return `<h5>${p1}</h5>`;
  });
  text = text.replace(/^## (.+)$/gm, (match, p1) => {
    console.log("Matched h2:", match);
    return `<h4>${p1}</h4>`;
  });
  text = text.replace(/^# (.+)$/gm, (match, p1) => {
    console.log("Matched h1:", match);
    return `<h3>${p1}</h3>`;
  });
  // Horizontal rule: ---
  text = text.replace(/^-{3,}$/gm, (match) => {
    console.log("Matched hr:", match);
    return '<hr>';
  });
  // Bold: **text**
  text = text.replace(/\*\*(.+?)\*\*/g, (match, p1) => {
    console.log("Matched bold:", match);
    return `<b>${p1}</b>`;
  });

  // Italic: *text*
  text = text.replace(/\*(.+?)\*/g, (match, p1) => {
    console.log("Matched italic:", match);
    return `<i>${p1}</i>`;
  });
  // Math: \[ ... \] (block)
  text = text.replace(/\\\[(.+?)\\\]/gs, (match, p1) => {
    console.log("Matched block math:", match);
    // Remove any stray \[ or \] inside p1
    const clean = p1.replace(/\\\[|\\\]/g, '').trim();
    return `<span class="math" data-display="block">${clean}</span>`;
  });
  // Math: \( ... \) (inline)
  text = text.replace(/\\\((.+?)\\\)/gs, (match, p1) => {
    console.log("Matched inline math:", match);
    return `<span class="math" data-display="inline">${p1}</span>`;
  });
  // Line breaks
  text = text.replace(/\n/g, "<br>");
  // Remove any line that is just \] or \[
  text = text.replace(/(<br>)*\\\](<br>)*/g, "");
  text = text.replace(/(<br>)*\\\[(<br>)*/g, "");
  // Remove multiple consecutive <br> (leave only one)
  text = text.replace(/(<br>\s*){2,}/g, "<br>");
  // Remove <br> right after block elements that already imply a newline
  text = text.replace(/(<\/h[1-6]>|<\/hr>|<\/p>|<\/table>|<\/ul>|<\/ol>|<\/pre>|<\/div>|<\/span>)(<br>)+/g, "$1");
  // Remove <br> right before a table
  text = text.replace(/(<br>\s*)+(<table)/g, "$2");
  // Remove leading <br> at the very start
  text = text.replace(/^(<br>\s*)+/, "");
  return text;
}

function autoWrapMath(text: string): string {
  // Only wrap lines that are just math, not sentences, and not already wrapped
  return text.replace(
    /^(?!\\\[)([0-9\.\+\-\*/\(\)\s×÷]+=[0-9\.\+\-\*/\(\)\s×÷]+)(?!\\\])$/gm,
    '\\[$1\\]'
  );
}

function renderFileList() {
  this._fileListRow.innerHTML = "";
  this._uploadedFiles.forEach(file => {
    const fileRow = document.createElement("div");
    fileRow.style.display = "flex";
    fileRow.style.alignItems = "center";
    fileRow.style.background = "#f6f8fa";
    fileRow.style.border = "1px solid #ccc";
    fileRow.style.borderRadius = "6px";
    fileRow.style.width = "auto"; // "100%";
    fileRow.style.height = "38px";
    fileRow.style.marginBottom = "4px";
    fileRow.style.padding = "0 8px";
    fileRow.style.fontSize = "15px";
    fileRow.style.justifyContent = "flex-start"; // "space-between";

    // SVG icon for file or image
    const iconSpan = document.createElement("span");
    iconSpan.style.display = "flex";
    iconSpan.style.alignItems = "center";
    iconSpan.style.justifyContent = "center";
    iconSpan.style.width = "24px";
    iconSpan.style.height = "24px";
    iconSpan.style.marginRight = "8px";
    iconSpan.innerHTML =
      file.type === "file"
        ? `<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="#0d6efd" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
            <path d="M9 17H15M9 13H15M9 9H10M13 3H8.2C7.0799 3 6.51984 3 6.09202 3.21799C5.71569 3.40973 5.40973 3.71569 5.21799 4.09202C5 4.51984 5 5.0799 5 6.2V17.8C5 18.9201 5 19.4802 5.21799 19.908C5.40973 20.2843 5.71569 20.5903 6.09202 20.782C6.51984 21 7.0799 21 8.2 21H15.8C16.9201 21 17.4802 21 17.908 20.782C18.2843 20.5903 18.5903 20.2843 18.782 19.908C19 19.4802 19 18.9201 19 17.8V9M13 3L19 9M13 3V7.4C13 7.96005 13 8.24008 13.109 8.45399C13.2049 8.64215 13.3578 8.79513 13.546 8.89101C13.7599 9 14.0399 9 14.6 9H19"/>
          </svg>`
        : file.type === "image"
        ? `<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="#0d6efd" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
            <path d="M8 11H10M8 18L11 15L13 17L16 14M13 3H8.2C7.0799 3 6.51984 3 6.09202 3.21799C5.71569 3.40973 5.40973 3.71569 5.21799 4.09202C5 4.51984 5 5.0799 5 6.2V17.8C5 18.9201 5 19.4802 5.21799 19.908C5.40973 20.2843 5.71569 20.5903 6.09202 20.782C6.51984 21 7.0799 21 8.2 21H15.8C16.9201 21 17.4802 21 17.908 20.782C18.2843 20.5903 18.5903 20.2843 18.782 19.908C19 19.4802 19 18.9201 19 17.8V9M13 3L19 9M13 3V7.4C13 7.96005 13 8.24008 13.109 8.45399C13.2049 8.64215 13.3578 8.79513 13.546 8.89101C13.7599 9 14.0399 9 14.6 9H19"/>
          </svg>`
        : `<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="#0d6efd" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
            <path d="M13 3H8.2C7.0799 3 6.51984 3 6.09202 3.21799C5.71569 3.40973 5.40973 3.71569 5.21799 4.09202C5 4.51984 5 5.0799 5 6.2V17.8C5 18.9201 5 19.4802 5.21799 19.908C5.40973 20.2843 5.71569 20.5903 6.09202 20.782C6.51984 21 7.0799 21 8.2 21H15.8C16.9201 21 17.4802 21 17.908 20.782C18.2843 20.5903 18.5903 20.2843 18.782 19.908C19 19.4802 19 18.9201 19 17.8V9M13 3L19 9M13 3V7.4C13 7.96005 13 8.24008 13.109 8.45399C13.2049 8.64215 13.3578 8.79513 13.546 8.89101C13.7599 9 14.0399 9 14.6 9H19M8 6H10M8 9H10M16 17H14L13 15.5L12 17L10 14L9.5 17H8"/>
          </svg>`;
    fileRow.appendChild(iconSpan);

    // File name (not clickable)
    const nameSpan = document.createElement("span");
    nameSpan.textContent = file.name;
    nameSpan.style.flex = "1";
    nameSpan.style.overflow = "hidden";
    nameSpan.style.textOverflow = "ellipsis";
    nameSpan.style.whiteSpace = "nowrap";
    fileRow.appendChild(nameSpan);

    // File size
    const sizeSpan = document.createElement("span");
    sizeSpan.textContent = `(${file.size})`;
    sizeSpan.style.color = "#aaa";
    sizeSpan.style.fontSize = "14px";
    sizeSpan.style.marginRight = "6px";
    fileRow.appendChild(sizeSpan);

    // Remove ("x") button
    const removeBtn = document.createElement("button");
    removeBtn.textContent = "✕";
    removeBtn.title = "Remove file";
    removeBtn.style.background = "transparent";
    removeBtn.style.border = "none";
    removeBtn.style.color = "#888";
    removeBtn.style.fontSize = "18px";
    removeBtn.style.cursor = "pointer";
    removeBtn.style.marginLeft = "8px";
    removeBtn.onclick = () => {
      // Remove by id
      this._uploadedFiles = this._uploadedFiles.filter(f => f.id !== file.id);
      renderFileList.call(this);
      // Log current file list with ids
      console.log("[File Removed] Current Files:\n", this._uploadedFiles.map(f => ({ id: f.id, name: f.name, type: f.type, size: f.size })));
    };
    fileRow.appendChild(removeBtn);

    this._fileListRow.appendChild(fileRow);
  });
  this._fileListRow.style.marginBottom = "4px";
  // Count files by type
  const fileCount = this._uploadedFiles.filter(f => f.type === "file").length;
  const imageCount = this._uploadedFiles.filter(f => f.type === "image").length;
  const screenshotCount = this._uploadedFiles.filter(f => f.type === "image-screenshot").length;
  // Update bubbles
  this._renderBubble(this._fileUploadBtn, fileCount, "bubble-upload-file");
  this._renderBubble(this._imageUploadBtn, imageCount, "bubble-upload-image");
  this._renderBubble(this._screenshotUploadBtn, screenshotCount, "bubble-upload-screenshot");
  // Log current file list with ids after every render
  console.log("[File Uploaded] Current Files:\n", this._uploadedFiles.map(f => ({ id: f.id, name: f.name, type: f.type, size: f.size })));
}

function formatFileSize(size: number): string {
  if (size < 1024) return `${size.toFixed(1)} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  return `${(size / (1024 * 1024)).toFixed(1)} MB`;
}
