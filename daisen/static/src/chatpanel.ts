import { sendGetGitHubIsAvailable, sendPostGPT, GPTRequest } from "./chatpanelrequests";
import katex from "katex";
import "katex/dist/katex.min.css";

export class ChatPanel {
  _chatMessages: { role: "user" | "assistant" | "system"; content: string }[] = [
    { role: "system", content: "You are Daisen Bot." }
  ];
  _uploadedFiles: { id: number; name: string; content: string; size: string }[] = [];
  _fileUploadBtn: HTMLButtonElement;
  _fileIdCounter: number = 0;
  _fileListRow: HTMLDivElement = document.createElement("div");
  _attachRepoVisible: boolean = false;
  _attachRepoChecks: { [key: string]: boolean } = {};
  _githubIsAvailableResponse: { available: number; routine_keys: string[] } | null = null;
  _chatPanel: HTMLDivElement | null = null;
  _chatPanelWidth: number = 0;
  protected _onChatPanelOpen() {}

  constructor() {
    this._fileIdCounter = 0;
    this._fileListRow = document.createElement("div");
    this._uploadedFiles = [];
    sendGetGitHubIsAvailable().then((resp) => {
      this._githubIsAvailableResponse = resp;
      console.log("[GitHubIsAvailableResponse]", resp);
    });
  }

  _showChatPanel() {
    // let messages: { role: "user" | "assistant" | "system"; content: string }[] = [
    //   { role: "system", content: "You are Daisen Bot."}
    // ];
    let messages = this._chatMessages;
    this._injectChatPanelCSS();

    // Remove existing panel if any
    let oldPanel = document.getElementById("chat-panel");
    if (oldPanel) oldPanel.remove();

    // Create the chat panel
    const chatPanel = document.createElement("div");
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

    // 
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

    // Get and update chat panel width after it's added to DOM
    setTimeout(() => {
      this._getChatPanelWidth();
    }, 10);

    // Force reflow to ensure the browser registers the new height before animating
    void chatPanel.offsetHeight;

    // Animate in
    chatPanel.classList.add('open');
    this._onChatPanelOpen();
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
    chatContent.style.flex = "1";
    chatContent.style.display = "flex";
    chatContent.style.flexDirection = "column";
    chatContent.style.padding = "20px";
    chatContent.style.minHeight = "0";
    chatPanel.appendChild(chatContent);
    
    this._chatPanel = chatPanel;

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
          userDiv.style.justifyContent = "flex-end";
          userDiv.style.margin = "4px 0";

          const userBubble = document.createElement("span");
          userBubble.innerHTML = "<b>You:</b> " + m.content;
          userBubble.style.background = "#0d6efd";
          userBubble.style.color = "white";
          userBubble.style.padding = "8px 12px";
          userBubble.style.borderRadius = "16px";
          userBubble.style.maxWidth = "90%";
          userBubble.style.display = "inline-block";
          userBubble.style.wordBreak = "break-word";
          userDiv.appendChild(userBubble);

          messagesDiv.appendChild(userDiv);
        } else if (m.role === "assistant") {
          const botDiv = document.createElement("div");
          botDiv.innerHTML = "<b>Daisen Bot:</b> " + convertMarkdownToHTML(autoWrapMath(m.content));
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

    const historyMenu = document.createElement("div");
    historyMenu.style.display = "flex";
    historyMenu.style.flexDirection = "column";
    historyMenu.style.marginBottom = "8px";
    chatContent.appendChild(historyMenu);

    function renderHistoryMenu() {
      const lastUserMessages = messages.filter(m => m.role === "user");

      // Remove uploaded files prefix if present
      const cleanedMessages = lastUserMessages.map(m => {
        const idx = m.content.indexOf("[End Uploaded Files]");
        if (idx !== -1) {
          // Remove everything up to and including "[End Uploaded Files]" and the next \n if present
          let after = m.content.slice(idx + "[End Uploaded Files]".length);
          if (after.startsWith("\n")) after = after.slice(1);
          return after;
        }
        return m.content;
      });

      // Remove duplicates, keep only the latest occurrence
      const seen = new Set<string>();
      const uniqueRecent: string[] = [];
      for (let i = cleanedMessages.length - 1; i >= 0 && uniqueRecent.length < 3; i--) {
        const msg = cleanedMessages[i];
        if (!seen.has(msg)) {
          seen.add(msg);
          uniqueRecent.unshift(msg); // Insert at the beginning to keep order
        }
      }
      
      historyMenu.innerHTML = "";
      uniqueRecent.forEach(msgContent => {
        const item = document.createElement("button");
        // Limit to 10 words for display
        const words = msgContent.split(" ");
        let displayText = msgContent;
        if (words.length > 10) {
          displayText = words.slice(0, 10).join(" ") + "...";
        }
        item.textContent = displayText;
        item.style.background = "#f8f9fa";
        item.style.border = "none";
        item.style.borderRadius = "6px";
        item.style.padding = "10px 16px";
        item.style.margin = "4px 0";
        item.style.fontSize = "1em";
        item.style.color = "#222";
        item.style.boxShadow = "0 2px 8px rgba(0,0,0,0.06)";
        item.style.cursor = "pointer";
        item.style.transition = "background 0.15s, box-shadow 0.15s";
        item.style.alignItems = "center";
        item.style.width = "auto";
        // Hover effect
        item.onmouseenter = () => {
          item.style.background = "#e9ecef";
        };
        item.onmouseleave = () => {
          item.style.background = "#f8f9fa";
        }; 

        // Fills input on click
        item.onclick = () => {
          input.value = msgContent;
          input.focus();
        };
        historyMenu.appendChild(item);
      });
    }

    // When panel opens
    renderHistoryMenu();

    // Initial welcome message
    const welcomeDiv = document.createElement("div");
    welcomeDiv.innerHTML = "<b>Daisen Bot:</b> Hello! What can I help you with today?";
    welcomeDiv.style.textAlign = "left";
    welcomeDiv.style.marginBottom = "8px";
    messagesDiv.appendChild(welcomeDiv);

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
        <path d="M14.3352 17.5003V15.6654H12.5002C12.1331 15.6654 11.8354 15.3674 11.8352 15.0003C11.8352 14.6331 12.133 14.3353 12.5002 14.3353H14.3352V12.5003C14.3352 12.1331 14.633 11.8353 15.0002 11.8353C15.3674 11.8355 15.6653 12.1332 15.6653 12.5003V14.3353H17.5002L17.634 14.349C17.937 14.411 18.1653 14.679 18.1653 15.0003C18.1651 15.3215 17.9369 15.5897 17.634 15.6517L17.5002 15.6654H15.6653V17.5003C15.6651 17.8673 15.3673 18.1652 15.0002 18.1654C14.6331 18.1654 14.3354 17.8674 14.3352 17.5003ZM16.0012 8.33333V7.33333C16.0012 6.62229 16.0013 6.12896 15.97 5.74544C15.9469 5.46349 15.9091 5.27398 15.8577 5.1302L15.802 5.00032C15.6481 4.69821 15.4137 4.44519 15.1262 4.26888L14.9993 4.19856C14.8413 4.11811 14.6297 4.06128 14.2542 4.03059C13.8707 3.99928 13.3772 3.99837 12.6663 3.99837H9.16431C9.16438 4.04639 9.15951 4.09505 9.14868 4.14388L8.61646 4.02571H8.61548L9.14868 4.14388L8.69263 6.19954C8.5874 6.67309 8.50752 7.06283 8.33911 7.3929L8.26196 7.53059C8.12314 7.75262 7.94729 7.94837 7.74341 8.11067L7.53052 8.26204C7.26187 8.42999 6.95158 8.52024 6.58521 8.60579L6.19946 8.6927L4.1438 9.14876L4.02564 8.61556V8.61653L4.1438 9.14876C4.09497 9.15959 4.04631 9.16446 3.99829 9.16438V12.6663C3.99829 13.3772 3.9992 13.8707 4.03052 14.2542C4.0612 14.6298 4.11803 14.8413 4.19849 14.9993L4.2688 15.1263C4.44511 15.4137 4.69813 15.6481 5.00024 15.8021L5.13013 15.8577C5.2739 15.9092 5.46341 15.947 5.74536 15.97C6.12888 16.0014 6.62221 16.0013 7.33325 16.0013H8.28442C8.65158 16.0013 8.94929 16.2992 8.94946 16.6663C8.94946 17.0336 8.65169 17.3314 8.28442 17.3314H7.33325C6.64416 17.3314 6.0872 17.332 5.63696 17.2952C5.23642 17.2625 4.87552 17.1982 4.53931 17.054L4.39673 16.9866C3.87561 16.7211 3.43911 16.3174 3.13501 15.8216L3.01294 15.6038C2.82097 15.2271 2.74177 14.8206 2.70435 14.3626C2.66758 13.9124 2.66821 13.3553 2.66821 12.6663V9.00032C2.66821 7.44077 3.58925 5.86261 4.76196 4.70638C5.9331 3.55172 7.50845 2.6675 9.00024 2.66927V2.66829H12.6663C13.3553 2.66829 13.9123 2.66765 14.3625 2.70442C14.8206 2.74184 15.227 2.82105 15.6038 3.01302L15.8215 3.13509C16.3174 3.43919 16.7211 3.87569 16.9866 4.39681L17.053 4.53938C17.1973 4.8757 17.2624 5.23636 17.2952 5.63704C17.332 6.08728 17.3313 6.64424 17.3313 7.33333V8.33333C17.3313 8.7006 17.0335 8.99837 16.6663 8.99837C16.2991 8.99819 16.0012 8.70049 16.0012 8.33333ZM7.76001 4.26204C7.06176 4.53903 6.3362 5.02201 5.69556 5.65364C5.04212 6.29794 4.53764 7.03653 4.25415 7.76204L5.91138 7.39388L6.30689 7.30403C6.62563 7.22833 6.73681 7.18952 6.82544 7.13411L6.91528 7.07063C7.00136 7.00214 7.07542 6.91924 7.13403 6.82552L7.18579 6.722C7.23608 6.59782 7.28758 6.38943 7.3938 5.91145L7.76001 4.26204Z" />
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
    chatContent.appendChild(actionRow);

    this._fileUploadBtn = fileUploadBtn;

    // Upload Daisen Trace button
    const traceUploadBtn = document.createElement("button");
    traceUploadBtn.type = "button";
    traceUploadBtn.title = "Upload Daisen Trace";
    traceUploadBtn.style.background = "#f6f8fa";
    traceUploadBtn.style.border = "1px solid #ccc";
    traceUploadBtn.style.borderRadius = "6px";
    traceUploadBtn.style.width = "38px";
    traceUploadBtn.style.height = "38px";
    traceUploadBtn.style.display = "flex";
    traceUploadBtn.style.alignItems = "center";
    traceUploadBtn.style.justifyContent = "center";
    traceUploadBtn.style.cursor = "pointer";
    traceUploadBtn.style.marginLeft = "4px";
    traceUploadBtn.innerHTML = `
      <svg width="24" height="24" viewBox="0 0 20 20" fill="currentColor">
        <path d="M11.5171 11.9924C11.5171 11.1544 10.8375 10.4749 9.99951 10.4748C9.1614 10.4748 8.48194 11.1543 8.48194 11.9924C8.48204 12.8304 9.16146 13.51 9.99951 13.51C10.8375 13.5099 11.517 12.8304 11.5171 11.9924ZM3.80713 8.71018C3.28699 8.92666 3.01978 9.50634 3.19385 10.0422L3.52686 11.0647L3.56494 11.1653C3.77863 11.6537 4.32351 11.9188 4.84717 11.7717L7.30322 11.0803C7.68362 9.95526 8.74606 9.14475 9.99951 9.14475C10.6946 9.14479 11.3313 9.39406 11.8257 9.80783L13.0933 9.45139L13.0278 9.28049L11.7671 5.40061L3.80713 8.71018ZM14.5962 3.05783L14.4683 3.08615L13.6382 3.35569C13.0705 3.54027 12.7594 4.15025 12.9438 4.71799L14.2935 8.86936L14.3325 8.97287C14.5537 9.47408 15.1235 9.73664 15.6558 9.56369L16.4858 9.29319L16.605 9.24045C16.8285 9.11361 16.9562 8.86404 16.9272 8.60861L16.8999 8.48166L15.2808 3.49924C15.1844 3.20321 14.894 3.02422 14.5962 3.05783ZM12.8472 11.9924C12.8471 12.9213 12.3997 13.7432 11.7114 14.2629L13.6187 17.3137L13.6782 17.4348C13.7863 17.7246 13.6802 18.0603 13.4077 18.2307C13.1352 18.401 12.7869 18.3493 12.5737 18.1252L12.4907 18.0188L10.4761 14.7961C10.3209 14.8223 10.1621 14.84 9.99951 14.8401C9.83597 14.8401 9.67603 14.8226 9.52002 14.7961L7.50635 18.0188L7.42334 18.1252C7.21015 18.3493 6.86184 18.401 6.58936 18.2307C6.27803 18.036 6.1838 17.6251 6.37842 17.3137L8.28467 14.2619C7.72379 13.8376 7.32535 13.2124 7.19776 12.4914L5.20752 13.052C4.03967 13.3804 2.82407 12.7896 2.34717 11.7004L2.26221 11.4758L1.9292 10.4533C1.54076 9.25785 2.13665 7.9642 3.29737 7.48166L11.5884 4.03342C11.7179 3.15622 12.3266 2.38379 13.2271 2.09104L14.0581 1.82053L14.2524 1.76779C15.2307 1.5561 16.2295 2.11668 16.5454 3.08908L18.1646 8.07053L18.2173 8.26584C18.4145 9.17874 17.9401 10.1097 17.0855 10.4865L16.897 10.5588L16.0659 10.8283C15.3549 11.0592 14.6144 10.9419 14.0288 10.5705L12.6499 10.9572C12.7754 11.2783 12.8472 11.6269 12.8472 11.9924Z"/>
      </svg>
    `;
    // Do nothing on click for now
    traceUploadBtn.onclick = () => {};

    actionRow.appendChild(traceUploadBtn);

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
      <svg width="24" height="24" viewBox="0 0 256 256" fill="currentColor">
        <path d="M69.12158,94.14551,28.49658,128l40.625,33.85449a7.99987,7.99987,0,1,1-10.24316,12.291l-48-40a7.99963,7.99963,0,0,1,0-12.291l48-40a7.99987,7.99987,0,1,1,10.24316,12.291Zm176,27.709-48-40a7.99987,7.99987,0,1,0-10.24316,12.291L227.50342,128l-40.625,33.85449a7.99987,7.99987,0,1,0,10.24316,12.291l48-40a7.99963,7.99963,0,0,0,0-12.291Zm-82.38769-89.373a8.00439,8.00439,0,0,0-10.25244,4.78418l-64,176a8.00034,8.00034,0,1,0,15.0371,5.46875l64-176A8.0008,8.0008,0,0,0,162.73389,32.48145Z"  />
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
    let repoRows = [];
    if (this._githubIsAvailableResponse && this._githubIsAvailableResponse.available === 1) {
      repoRows = [...this._githubIsAvailableResponse.routine_keys].sort();
    }

    // repoRows.forEach(row => {
    //   const rowDiv = document.createElement("div");
    //   rowDiv.style.display = "flex";
    //   rowDiv.style.alignItems = "center";
    //   rowDiv.style.justifyContent = "space-between";
    //   rowDiv.style.marginBottom = "8px";

    //   const label = document.createElement("span");
    //   label.textContent = row;
    //   label.style.fontSize = "15px";
    //   label.style.color = "#222";
    //   label.style.maxWidth = "270px";
    //   label.style.overflow = "hidden";
    //   label.style.textOverflow = "ellipsis";
    //   label.style.whiteSpace = "nowrap";

    //   const checkbox = document.createElement("input");
    //   checkbox.type = "checkbox";
    //   checkbox.checked = (row in this._attachRepoChecks) ? this._attachRepoChecks[row] : false;
    //   checkbox.onchange = () => {
    //     this._attachRepoChecks[row] = checkbox.checked;
    //     const checkedCount = Object.values(this._attachRepoChecks).filter(Boolean).length;
    //     this._renderBubble(attachRepoBtn, checkedCount, "bubble-attach-repo");
    //   };
    //   checkbox.style.marginLeft = "8px";

    //   rowDiv.appendChild(label);
    //   rowDiv.appendChild(checkbox);
    //   attachRepoDiv.appendChild(rowDiv);
    // });
    // Top row: Select All / Deselect All
    const topRow = document.createElement("div");
    topRow.style.display = "flex";
    topRow.style.justifyContent = "flex-start";
    topRow.style.alignItems = "center";
    topRow.style.marginBottom = "8px";

    const selectAllBtn = document.createElement("button");
    selectAllBtn.style.marginRight = "8px";
    selectAllBtn.textContent = "Select All";
    selectAllBtn.style.background = "#0d6efd";
    selectAllBtn.style.color = "#fff";
    selectAllBtn.style.border = "none";
    selectAllBtn.style.borderRadius = "4px";
    selectAllBtn.style.padding = "4px 10px";
    selectAllBtn.style.cursor = "pointer";
    selectAllBtn.style.fontSize = "13px";

    const deselectAllBtn = document.createElement("button");
    deselectAllBtn.textContent = "Deselect All";
    deselectAllBtn.style.background = "#6c757d";
    deselectAllBtn.style.color = "#fff";
    deselectAllBtn.style.border = "none";
    deselectAllBtn.style.borderRadius = "4px";
    deselectAllBtn.style.padding = "4px 10px";
    deselectAllBtn.style.cursor = "pointer";
    deselectAllBtn.style.fontSize = "13px";

    topRow.appendChild(selectAllBtn);
    topRow.appendChild(deselectAllBtn);
    attachRepoDiv.appendChild(topRow);

    // Scrollable region for checkboxes
    const scrollRegion = document.createElement("div");
    scrollRegion.style.maxHeight = "300px";
    scrollRegion.style.overflowY = "auto";
    scrollRegion.style.paddingRight = "4px";

    // Store checkbox elements for easy access
    const checkboxMap: { [key: string]: HTMLInputElement } = {};

    repoRows.forEach(row => {
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

      const checkbox = document.createElement("input");
      checkbox.type = "checkbox";
      checkbox.checked = (row in this._attachRepoChecks) ? this._attachRepoChecks[row] : false;
      checkbox.onchange = () => {
        this._attachRepoChecks[row] = checkbox.checked;
        const checkedCount = Object.values(this._attachRepoChecks).filter(Boolean).length;
        this._renderBubble(attachRepoBtn, checkedCount, "bubble-attach-repo");
      };
      checkbox.style.marginLeft = "8px";
      checkboxMap[row] = checkbox;

      rowDiv.appendChild(label);
      rowDiv.appendChild(checkbox);
      scrollRegion.appendChild(rowDiv);
    });

    attachRepoDiv.appendChild(scrollRegion);

    // Select All / Deselect All logic
    selectAllBtn.onclick = () => {
      repoRows.forEach(row => {
        this._attachRepoChecks[row] = true;
        checkboxMap[row].checked = true;
      });
      const checkedCount = repoRows.length;
      this._renderBubble(attachRepoBtn, checkedCount, "bubble-attach-repo");
    };

    deselectAllBtn.onclick = () => {
      repoRows.forEach(row => {
        this._attachRepoChecks[row] = false;
        checkboxMap[row].checked = false;
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
        // const currentWidth = attachRepoDiv.offsetWidth;
        // attachRepoDiv.style.width = (currentWidth + 10) + "px";
        attachRepoBtn.style.background = "#0d6efd";
        attachRepoBtn.style.color = "#fff";
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
          <path d="M69.12158,94.14551,28.49658,128l40.625,33.85449a7.99987,7.99987,0,1,1-10.24316,12.291l-48-40a7.99963,7.99963,0,0,1,0-12.291l48-40a7.99987,7.99987,0,1,1,10.24316,12.291Zm176,27.709-48-40a7.99987,7.99987,0,1,0-10.24316,12.291L227.50342,128l-40.625,33.85449a7.99987,7.99987,0,1,0,10.24316,12.291l48-40a7.99963,7.99963,0,0,0,0-12.291Zm-82.38769-89.373a8.00439,8.00439,0,0,0-10.25244,4.78418l-64,176a8.00034,8.00034,0,1,0,15.0371,5.46875l64-176A8.0008,8.0008,0,0,0,162.73389,32.48145Z"/>
          <line x1="6" y1="6" x2="18" y2="18" stroke="#e53935" stroke-width="2"/>
        </svg>
      `;
    }

    actionRow.appendChild(attachRepoBtn);

    // Initial bubble for file upload button
    this._renderBubble(fileUploadBtn, this._uploadedFiles.length, "bubble-upload-file");

    // Initial bubble for attach repo button
    const checkedCount = Object.values(this._attachRepoChecks).filter(Boolean).length;
    this._renderBubble(attachRepoBtn, checkedCount, "bubble-attach-repo");


    // Input area
    const inputContainer = document.createElement("div");
    inputContainer.style.display = "flex";
    inputContainer.style.gap = "8px";

    const input = document.createElement("textarea");
    input.placeholder = "Type a message...";
    input.rows = 1;
    input.style.flex = "1";
    input.style.padding = "6px";
    input.style.borderRadius = "4px";
    input.style.border = "1px solid #ccc";
    input.style.resize = "none";
    input.style.overflowY = "auto";
    input.style.minHeight = "38px";
    input.style.maxHeight = "130px";

    // Auto-resize as user types
    input.addEventListener("input", function() {
      this.style.height = "auto";
      this.style.height = (this.scrollHeight) + "px";
    });

    const sendBtn = document.createElement("button");
    sendBtn.textContent = "Send";
    sendBtn.className = "btn btn-primary";

    const clearBtn = document.createElement("button");
    clearBtn.textContent = "New Chat";
    clearBtn.className = "btn btn-secondary";
    // clearBtn.style.marginLeft = "4px";

    // Send handler
    const sendMessage = () => {
      const userMsg = input.value.trim();
      if (!userMsg) return;

      // Build uploaded files prefix if any
      let prefix = "";
      if (this._uploadedFiles.length > 0) {
        prefix += "[Uploaded Files]\n";
        this._uploadedFiles.forEach(f => {
          prefix += `[Uploaded File "${f.name}"]\n${f.content}\n`;
        });
        prefix += "[End Uploaded Files]\n";
      }

      // Compose the full message
      const fullMsg = prefix + userMsg;

      // Disable send button while waiting
      sendBtn.disabled = true;
      input.disabled = true;

      // User message
      const userDiv = document.createElement("div");
      userDiv.style.display = "flex";
      userDiv.style.justifyContent = "flex-end";
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

      messagesDiv.appendChild(userDiv);

      // Call GPT with full history
      messages.push({ role: "user", content: fullMsg });
      console.log("[Sent to GPT]", fullMsg);

      // Show history menu
      renderHistoryMenu();
      
      // Clear input field
      input.value = "";

      // Show "thinking message"
      const botDiv = document.createElement("div");
      botDiv.innerHTML = `<b>Daisen Bot:</b> <i>Thinking<span id="thinking-dots">.</span></i>`;
      // botDiv.innerHTML = "<b>Daisen Bot:</b> <i>Thinking...</i>";;
      botDiv.style.textAlign = "left";
      botDiv.style.margin = "4px 0";
      messagesDiv.appendChild(botDiv);

      let dotCount = 1;
      const maxDots = 3;
      const thinkingDots = botDiv.querySelector("#thinking-dots");
      const dotsInterval = setInterval(() => {
        dotCount = (dotCount % maxDots) + 1;
        if (thinkingDots) thinkingDots.textContent = ".".repeat(dotCount);
      }, 500);

      // Call GPT and update the message
      const selectedGitHubRoutineKeys = Object.keys(this._attachRepoChecks).filter(k => this._attachRepoChecks[k]);
      const gptRequest: GPTRequest = {
        messages: messages,
        traceInfo: {
            selected: 0,
            starttime: 0,
            endtime: 0.00008
        },
        selectedGitHubRoutineKeys: selectedGitHubRoutineKeys
      };
      console.log("GPTRequest:", gptRequest);
      
      sendPostGPT(gptRequest).then((gptResponse) => {
        const gptResponseContent = gptResponse.content;
        const gptResponseTotalTokens = gptResponse.totalTokens;
        console.log("[Received from GPT - Cost] Total tokens used:", gptResponseTotalTokens !== -1 ? gptResponseTotalTokens : "unknown");
        botDiv.innerHTML = `<b>Daisen Bot:</b> <span style="color:#aaa;font-size:0.95em;">(${gptResponseTotalTokens.toLocaleString()} tokens)</span> ` + convertMarkdownToHTML(autoWrapMath(gptResponseContent));

        messages.push({ role: "assistant", content: gptResponseContent });
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
        if (checkboxMap[key]) checkboxMap[key].checked = false;
      });
      this._attachRepoVisible = false;
      attachRepoDiv.style.display = "none";
      attachRepoBtn.style.background = "#f6f8fa";
      attachRepoBtn.style.color = "#222";
      this._renderBubble(attachRepoBtn, 0, "bubble-attach-repo");
    }

    sendBtn.onclick = sendMessage;
    input.addEventListener("keydown", (e) => {
      if (e.key === "Enter" && !e.shiftKey) {
        e.preventDefault();
        sendMessage();
      }
    });

    clearBtn.onclick = () => {
      messages.length = 0;
      messages.push({ role: "system", content: "You are Daisen Bot." });
      input.value = "";
      // Remove all messages from the chat panel except the welcome message
      messagesDiv.innerHTML = "";
      const welcomeDiv = document.createElement("div");
      welcomeDiv.innerHTML = "<b>Daisen Bot:</b> Hello! What can I help you with today?";
      welcomeDiv.style.textAlign = "left";
      welcomeDiv.style.marginBottom = "8px";
      messagesDiv.appendChild(welcomeDiv);
      renderHistoryMenu();
      input.style.height = "38px";
    };

    inputContainer.appendChild(input);
    inputContainer.appendChild(sendBtn);
    inputContainer.appendChild(clearBtn);
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
      bubble.style.fontSize = "12px";
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
    } else if (value > 9) {
      bubble.style.opacity = "1";
      bubble.textContent = "9+";
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
      console.log("[File Removed] Current Files:\n", this._uploadedFiles.map(f => ({ id: f.id, name: f.name, size: f.size })));
    };
    fileRow.appendChild(removeBtn);

    this._fileListRow.appendChild(fileRow);
  });
  this._fileListRow.style.marginBottom = "4px";
  // Update bubble
  this._renderBubble(this._fileUploadBtn, this._uploadedFiles.length, "bubble-upload-file");
  // Log current file list with ids after every render
  console.log("[File Uploaded] Current Files:\n", this._uploadedFiles.map(f => ({ id: f.id, name: f.name, size: f.size })));
}

function formatFileSize(size: number): string {
  if (size < 1024) return `${size.toFixed(1)} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  return `${(size / (1024 * 1024)).toFixed(1)} MB`;
}
