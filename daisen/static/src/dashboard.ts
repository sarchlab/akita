import * as d3 from "d3";
import Widget from "./widget";
import { thresholdFreedmanDiaconis } from "d3";
import { sendPostGPT } from "./sendPostGPT";
import katex from "katex";
import "katex/dist/katex.min.css";

class YAxisOption {
  optionValue: string;
  html: string;
}

class Dashboard {
  _canvas: HTMLDivElement;
  _pageBtnContainer: HTMLDivElement;
  _toolBar: HTMLFormElement;

  _componentNames: Array<string>;
  _filteredNames: Array<string>;
  _numWidget: number;
  _numRow: number;
  _numCol: number;
  _currPage: number;
  _currFilter: string;
  _filterTimer: NodeJS.Timeout;
  _primaryAxis: string;
  _secondaryAxis: string;
  _startTime: number;
  _endTime: number;
  _widgets: Array<Widget>;
  _yAxisOptions: Array<YAxisOption>;
  _initialWidth: number;
  _initialHeight: number;
  _burgerMenu: HTMLDivElement;
  _dropdownCanvas: HTMLDivElement;
  _showChatButton: boolean = true; // Add this flag to control the right chat button visibility
  _chatMessages: { role: "user" | "assistant" | "system"; content: string }[] = [
    { role: "system", content: "You are Daisen Bot." }
  ];  // Make the message history global
  _uploadedFiles: { id: number; name: string; content: string; size: string }[] = [];
  _fileIdCounter: number;
  _fileListRow: HTMLDivElement; // Add this to hold the file list container

  constructor() {
    this._numWidget = 16;
    this._numRow = 4;
    this._numCol = 4;
    this._widgets = [];
    this._yAxisOptions = [
      { optionValue: "ReqInCount", html: "Incoming Request Rate" },
      { optionValue: "ReqCompleteCount", html: "Request Complete Rate" },
      { optionValue: "AvgLatency", html: "Average Request Latency" },
      { optionValue: "ConcurrentTask", html: "Number Concurrent Task" },
      { optionValue: "BufferPressure", html: "Buffer Pressure" },
      { optionValue: "PendingReqOut", html: "Pending Request Out" },
      { optionValue: "-", html: " - " },
    ];
    this._initialWidth = window.innerWidth;
    this._initialHeight = window.innerHeight;
    this._fileIdCounter = 0;
    this._fileListRow = document.createElement("div");
    this._uploadedFiles = [];
  }

  setCanvas(
    canvas: HTMLDivElement,
    pageBtnContainer: HTMLDivElement,
    toolBar: HTMLFormElement
  ) {
    this._canvas = canvas;
    this._pageBtnContainer = pageBtnContainer;
    this._toolBar = toolBar;
  
    this._canvas.classList.add('canvas-container');
    
    if (this._burgerMenu) {
      this._burgerMenu.remove();
    }
    if (this._dropdownCanvas) {
      this._dropdownCanvas.remove();
    }
    
    this._burgerMenu = document.createElement('div');
    this._burgerMenu.classList.add('burger-menu');
    this._burgerMenu.innerHTML = `
      <div class="burger-bar"></div>
      <div class="burger-bar"></div>
      <div class="burger-bar"></div>
    `;
    this._burgerMenu.style.position = 'absolute';
    this._burgerMenu.style.top = '10px';
    this._burgerMenu.style.right = '10px';

    this._dropdownCanvas = document.createElement('div');
    this._dropdownCanvas.classList.add('dropdown-canvas');
    this._dropdownCanvas.style.display = 'none';

    document.body.appendChild(this._burgerMenu);
    document.body.appendChild(this._dropdownCanvas);
  
    this._burgerMenu.addEventListener('click', () => {
      const isActive = this._dropdownCanvas.classList.toggle('active');
      this._dropdownCanvas.style.display = isActive ? 'block' : 'none';
    });
  
    window.addEventListener('resize', () => {
      this._updateNavbarVisibility();
      this._resize();
      this._widgets.forEach((w: Widget) => {
        w.createWidget(this._widgetWidth(), this._widgetHeight());
        w.setXAxis(this._startTime, this._endTime);
        w.setFirstAxis(this._primaryAxis);
        w.setSecondAxis(this._secondaryAxis);
      });
      const paginationContainer = this._canvas.querySelector('.pagination-container') as HTMLElement;
      if (paginationContainer) {
        paginationContainer.style.left = '50%';
        paginationContainer.style.transform = 'translateX(-50%)';
      }
      this.render();
    }); 

    this._addZoomResetButton(this._toolBar);
    this._addFilterUI(this._toolBar);
    this._addPrimarySelector(this._toolBar);
    this._addSecondarySelector(this._toolBar);
    this._addZoomResetButton(this._dropdownCanvas);
    this._addFilterUI(this._dropdownCanvas);
    this._addPrimarySelector(this._dropdownCanvas);
    this._addSecondarySelector(this._dropdownCanvas);
    this._resize();

  }

  _updateNavbarVisibility() {
    if (window.innerWidth <= 1365) {
      this._toolBar.style.display = 'none';
      this._burgerMenu.style.display = 'block';
    } else {
      this._toolBar.style.display = 'flex';
      this._burgerMenu.style.display = 'none';
      this._dropdownCanvas.style.display = 'none';
      this._dropdownCanvas.classList.remove('active');
    }
  }

  _resize() {
    this._resetNumRowCol();
    const width = this._widgetWidth();
    const height = this._widgetHeight();
    this._widgets.forEach((w: Widget) => {
      w.resize(width, height);
    });
  }

  _resetNumRowCol() {
    const rowColTable = [
      [0, 0],
      [1, 1],
      [2, 1],
      [2, 2],
      [2, 2],
      [2, 3],
      [2, 3],
      [3, 3],
      [3, 3],
      [3, 3],
      [3, 4],
      [3, 4],
      [3, 4],
      [4, 4],
      [4, 4],
      [4, 4],
      [4, 4],
    ];
    const width = window.innerWidth;
    const height = window.innerHeight;
    this._numCol = rowColTable[this._numWidget][0];
    this._numRow = rowColTable[this._numWidget][1];
    if (width >= 1200) {
      this._numCol = 4;
    }
    if (width < 1200 && width >= 800) {
      this._numCol = 3;
    }
    if (width < 800) {
      this._numCol = 2;
    }
    console.log(width, height);
  }

  _widgetWidth(): number {
    this._resetNumRowCol();
    const numGap = this._numCol + 1;
    const marginLeft = 5;
    const gapSpace = numGap * marginLeft;
    const widgetSpace = this._canvas.offsetWidth - gapSpace;
    const width = Math.floor(widgetSpace / this._numCol);
    return width;
  }

  _widgetHeight(): number {
    this._resetNumRowCol();
    const numGap = this._numRow + 1;
    const marginTop = 5;
    const gapSpace = numGap * marginTop;
    const widgetSpace = this._canvas.offsetHeight - gapSpace;
    const height = Math.floor(widgetSpace / this._numRow);
    return height;
  }

  async _resetZoom() {
    let startTime = 0;
    let endTime = 0;
    await fetch("/api/trace?kind=Simulation")
      .then((rsp) => rsp.json())
      .then((rsp) => {
        if (rsp[0]) {
          startTime = rsp[0].start_time;
          endTime = rsp[0].end_time;
        } else {
          startTime = 0;
          endTime = 0.000001;
        }
      });
    this.setTimeRange(startTime, endTime);
  }

  setTimeRange(startTime: number, endTime: number) {
    this._startTime = startTime;
    this._endTime = endTime;

    const params = new URLSearchParams(window.location.search);
    params.set("starttime", startTime.toString());
    params.set("endtime", endTime.toString());
    window.history.replaceState(null, null, `/dashboard?${params.toString()}`);

    this._widgets.forEach((w) => {
      w.temporaryTimeShift(startTime, endTime);
      w.render(false);
    });
  }

  _addZoomResetButton(container: HTMLElement) {
    const btn = document.createElement('button');
    btn.setAttribute('type', 'button');
    btn.classList.add('btn', 'btn-primary', 'mr-3');
    btn.id = "dashboard-btn";
    btn.innerHTML = 'Reset Zoom';
    btn.onclick = () => {
      this._resetZoom();
    };
    container.appendChild(btn);
  }
  
  _addFilterUI(container: HTMLElement) {
    const filterGroup = document.createElement('div');
    filterGroup.classList.add('input-group', 'mr-3');
    filterGroup.id = 'dashboard-filter-group';
    filterGroup.innerHTML = `
      <div class="input-group-prepend">
        <span class="input-group-text" id="basic-addon1">Filter</span>
      </div>
      <input id="dashboard-filter-input" type="text" class="form-control" 
        placeholder="Component Name" aria-label="Filter" aria-describedby="basic-addon1">
    `;
    container.appendChild(filterGroup);
  
    const input = filterGroup.getElementsByTagName('input')[0];
    input.oninput = () => {
      this._filterInputChage();
    };
  }
  
  _addPrimarySelector(container: HTMLElement) {
    const selectorGroup = document.createElement('div');
    selectorGroup.classList.add('input-group', 'ml-3');
    const button = document.createElement('div');
    button.classList.add('input-group-prepend');
    selectorGroup.id = 'primary-selector-group'; 
    button.innerHTML = `
      <label class="input-group-text" for="primary-axis-select">
        <div class="circle-primary"></div>
        <div class="circle-primary"></div>
        <div class="circle-primary"></div>
        Primary Y-Axis
      </label>`;
    selectorGroup.appendChild(button);
  
    const select = document.createElement('select');
    select.classList.add('custom-select');
    select.id = 'primary-axis-select';
    selectorGroup.appendChild(select);
  
    this._yAxisOptions.forEach((o, index) => {
      let option = document.createElement('option');
      if (index === 0) {
        option.selected = true;
      }
      option.value = o.optionValue;
      option.innerHTML = o.html;
      select.add(option);
    });
  
    select.onchange = () => {
      this._switchPrimaryAxis(select.value);
    };
  
    container.appendChild(selectorGroup);
    this._canvas.classList.add('canvas-container');
  }
  
  _addSecondarySelector(container: HTMLElement) {
    const selectorGroup = document.createElement('div');
    selectorGroup.classList.add('input-group', 'ml-3');
    const button = document.createElement('div');
    button.classList.add('input-group-prepend');
    selectorGroup.id = 'secondary-selector-group';
    button.innerHTML = `
      <label class="input-group-text" for="secondary-axis-select">
        <div class="circle-secondary"></div>
        <div class="circle-secondary"></div>
        <div class="circle-secondary"></div>
        Secondary Y-Axis
      </label>`;
    selectorGroup.appendChild(button);
  
    const select = document.createElement('select');
    select.classList.add('custom-select');
    select.id = 'secondary-axis-select';
    selectorGroup.appendChild(select);
  
    this._yAxisOptions.forEach((o, index) => {
      let option = document.createElement('option');
      if (index === 0) {
        option.selected = true;
      }
      option.value = o.optionValue;
      option.innerHTML = o.html;
      select.add(option);
    });
  
    select.onchange = () => {
      this._switchSecondaryAxis(select.value);
    };
  
    container.appendChild(selectorGroup);
  }
  

  _switchPrimaryAxis(name: string) {
    this._primaryAxis = name;
    this._widgets.forEach((w) => {
      const params = new URLSearchParams(window.location.search);
      params.set("primary_y", name);
      window.history.pushState(null, null, `/dashboard?${params.toString()}`);

      w.setFirstAxis(name);
      w.render(true);
    });
  }

  _switchSecondaryAxis(name: string) {
    this._secondaryAxis = name;
    this._widgets.forEach((w) => {
      const params = new URLSearchParams(window.location.search);
      params.set("second_y", name);
      window.history.pushState(null, null, `/dashboard?${params.toString()}`);

      w.setSecondAxis(name);
      w.render(true);
    });
  }

  _filterInputChage() {
    window.clearTimeout(this._filterTimer);
    this._filterTimer = setTimeout(() => {
      const filterInput = <HTMLInputElement>(
        document.getElementById("dashboard-filter-input")
      );
      const filterString = filterInput.value;
      this._currPage = 0;

      const params = new URLSearchParams(window.location.search);
      params.set("filter", filterString);
      params.set("page", "0");
      window.history.pushState(null, null, `/dashboard?${params.toString()}`);

      this._currFilter = filterString;
      this._filter();
    }, 1000);
  }

  render() {
    Promise.all([fetch("/api/trace?kind=Simulation"), fetch("/api/compnames")])
      .then(([simulation, compNames]) => {
        return Promise.all([simulation.json(), compNames.json()]);
      })
      .then(([simulation, compNames]) => {
        simulation = simulation[0];

        compNames.sort();

        this._componentNames = compNames;
        this._filteredNames = compNames;

        this._getParamsFromURL(simulation);
        this._updateNavbarVisibility();
        this._filter();
      });
  }

  _getParamsFromURL(simulation: Object) {
    const params = new URLSearchParams(window.location.search);
    let page = params.get("page");
    let pageNum = 0;
    if (page === null) {
      pageNum = 0;
    } else {
      pageNum = parseInt(page);
    }

    let filter = params.get("filter");
    if (filter === null) {
      filter = "";
    }
    const dashboardFilter = <HTMLInputElement>(
      document.getElementById("dashboard-filter-input")
    );
    dashboardFilter.value = filter;

    let primaryAxisName = params.get("primary_y");
    if (primaryAxisName === null) {
      primaryAxisName = "ReqInCount";
    }
    const primaryAxisSelect = <HTMLInputElement>(
      document.getElementById("primary-axis-select")
    );
    primaryAxisSelect.value = primaryAxisName;

    let secondaryAxisName = params.get("second_y");
    if (secondaryAxisName === null) {
      secondaryAxisName = "AvgLatency";
    }
    const secondaryAxisSelect = <HTMLInputElement>(
      document.getElementById("secondary-axis-select")
    );
    secondaryAxisSelect.value = secondaryAxisName;

    let startTime = params.get("starttime");
    let endTime = params.get("endtime");
    if (startTime !== null && endTime !== null) {
      this.setTimeRange(parseFloat(startTime), parseFloat(endTime));
    } else if (!simulation) {
      this.setTimeRange(0, 0.000001);
    } else {
      this.setTimeRange(simulation["start_time"], simulation["end_time"]);
    }

    this._currPage = pageNum;
    this._currFilter = filter;
    this._primaryAxis = primaryAxisName;
    this._secondaryAxis = secondaryAxisName;
  }

  _filter() {
    if (this._currFilter === "") {
      this._filteredNames = this._componentNames;
    } else {
      const re = new RegExp(this._currFilter);
      this._filteredNames = this._componentNames.filter(
        (v) => v.search(re) >= 0
      );
    }

    this._numWidget = this._filteredNames.length;
    if (this._numWidget > 16) {
      this._numWidget = 16;
    }
    this._resetNumRowCol();

    this._addPaginationControl();
    this._renderPage();
  }

  _addPaginationControl() {
      const nav = document.createElement("nav");
      nav.classList.add("mt-4");
      const ul = document.createElement("ul");
      ul.classList.add("pagination");
      nav.appendChild(ul);

      const pageInfo = document.createElement("div");
      pageInfo.classList.add("page-info");

      // Create the right button
      const chatButton = document.createElement("button");
      chatButton.classList.add("btn", "btn-secondary", "ml-3");
      chatButton.style.display = "flex";
      chatButton.style.alignItems = "center";
      chatButton.style.paddingRight = "20px";
      chatButton.style.marginRight = "15px";
      chatButton.style.backgroundColor = "#0d6efd";
      chatButton.style.borderColor = "#0d6efd";
      chatButton.innerText = "Daisen Bot";
      chatButton.innerHTML = `
        <span style="display:inline-block;width:30px;height:30px;margin-right:px;">
          <svg width="30" height="30" viewBox="0 0 60 60" xmlns="http://www.w3.org/2000/svg">
            <!-- Central sparkle border -->
            <path d="M30,12 L29,13 L29,15 L28,16 L28,17 L27,18 L27,20 L26,21 L26,22 L24,24 L23,24 L22,25 L21,25 L20,26 L19,26 L18,27 L16,27 L15,28 L14,28 L13,29 L13,30 L14,31 L16,31 L17,32 L18,32 L19,33 L21,33 L22,34 L23,34 L25,36 L25,37 L26,38 L26,39 L27,40 L27,41 L28,42 L28,44 L29,45 L29,46 L30,47 L31,47 L32,46 L32,44 L33,43 L33,42 L34,41 L34,39 L35,38 L35,37 L37,35 L38,35 L39,34 L40,34 L41,33 L42,33 L43,32 L45,32 L46,31 L47,31 L48,30 L48,29 L47,28 L45,28 L44,27 L43,27 L42,26 L40,26 L39,25 L38,25 L36,23 L36,22 L35,21 L35,20 L34,19 L34,18 L33,17 L33,15 L32,14 L32,13 L31,12 Z"
                  fill="none" stroke="white" stroke-width="2" />

            <!-- Top-right "+" sign -->
            <line x1="44" y1="10" x2="44" y2="18" stroke="white" stroke-width="2"/>
            <line x1="40" y1="14" x2="48" y2="14" stroke="white" stroke-width="2"/>

            <!-- Bottom-left "+" sign -->
            <line x1="16" y1="42" x2="16" y2="50" stroke="white" stroke-width="2"/>
            <line x1="12" y1="46" x2="20" y2="46" stroke="white" stroke-width="2"/>
          </svg>
        </span>
        Daisen Bot
      `;
      chatButton.style.visibility = this._showChatButton ? "visible" : "hidden";
      chatButton.onclick = () => {
        this._showChatPanel();
      };

      // Pagination container with flexbox
      const paginationContainer = document.createElement("div");
      paginationContainer.classList.add("pagination-container");
      paginationContainer.style.display = "flex";
      paginationContainer.style.alignItems = "center";
      paginationContainer.style.justifyContent = "space-between";
      paginationContainer.style.position = "relative";
      paginationContainer.style.width = "100%";
      paginationContainer.style.minHeight = "60px";

      // Left: nav + pageInfo, Right: button
      const leftContainer = document.createElement("div");
      leftContainer.style.display = "flex";
      leftContainer.style.alignItems = "center";
      leftContainer.style.flex = "1";
      leftContainer.style.justifyContent = "center";
      leftContainer.appendChild(nav);
      leftContainer.appendChild(pageInfo);

      // When chat panel is open, shift leftContainer right by 600px
      if (!this._showChatButton) {
        leftContainer.style.marginLeft = "600px";
      } else {
        leftContainer.style.marginLeft = "0";
      }

      // Absolutely position the chat button to the right
      chatButton.style.position = "absolute";
      chatButton.style.right = "0";
      chatButton.style.top = "50%";
      chatButton.style.transform = "translateY(-50%)";
      chatButton.style.zIndex = "2";

      paginationContainer.appendChild(leftContainer);
      paginationContainer.appendChild(chatButton);

      if (this._canvas.querySelector('.pagination-container')) {
        this._canvas.removeChild(this._canvas.querySelector('.pagination-container'));
      }
      this._canvas.appendChild(paginationContainer);
      this._addPageButtons(ul);
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

    // Store the original width before shrinking
    const canvasContainer = this._canvas;
    let originalCanvasWidth = "";
    if (canvasContainer) {
      originalCanvasWidth = canvasContainer.style.width;
      canvasContainer.style.transition = "width 0.3s cubic-bezier(.4,0,.2,1)";
      canvasContainer.style.width = "calc(100% - 600px)";
      setTimeout(() => {
        this._resize();
        this._renderPage();
      }, 300);
    }

    // Triangle close button
    const closeBtn = document.createElement("button");
    closeBtn.style.position = "absolute";
    closeBtn.style.left = "-4px";
    closeBtn.style.top = "50%";
    closeBtn.style.transform = "translateY(-50%)";
    closeBtn.style.width = "12px";
    closeBtn.style.height = "40px";
    closeBtn.style.background = "transparent";
    closeBtn.style.border = "none";
    closeBtn.style.cursor = "pointer";
    closeBtn.title = "Close";

    closeBtn.innerHTML = `
      <svg width="12" height="40" viewBox="0 0 12 40" xmlns="http://www.w3.org/2000/svg">
        <defs>
          <linearGradient id="silverGradient" x1="0" y1="0" x2="1" y2="0">
            <stop offset="0%" stop-color="#d9d9d9"/>
            <stop offset="50%" stop-color="#b0b0b0"/>
            <stop offset="100%" stop-color="#f5f5f5"/>
          </linearGradient>
        </defs>
        <polygon points="0,0 12,20 0,40" fill="url(#silverGradient)" stroke="#aaa" stroke-width="0.5"/>
      </svg>
    `;

    const handleResize = () => {
      //Adjust chat panel height and top
      const innerContainer = document.getElementById("inner-container");
      if (innerContainer) {
        const rect = innerContainer.getBoundingClientRect();
        chatPanel.style.top = rect.top + "px";
        chatPanel.style.height = rect.height + "px";
      } else {
        chatPanel.style.top = "0";
        chatPanel.style.height = "100vh";
      }
      // Shrink canvas again (in case window size changed)
      if (canvasContainer) {
        canvasContainer.style.width = "calc(100% - 600px)";
      }
      // Re-render widgets
      this._resize();
      this._renderPage();
    }

    window.addEventListener("resize", handleResize);

    closeBtn.onclick = () => {
      // Animate out
      chatPanel.classList.remove('open');
      chatPanel.classList.add('closing');
      setTimeout(() => {
        chatPanel.remove();
        window.removeEventListener("resize", handleResize);
        this._showChatButton = true;
        this._addPaginationControl();
        // Restore the canvas container width to its original value
        if (canvasContainer) {
          canvasContainer.style.width = originalCanvasWidth || "100%";
          setTimeout(() => {
            this._resize();
            this._renderPage();
          }, 300);
        }
      }, 200); // Match the CSS transition duration
    };

    chatPanel.appendChild(closeBtn);

    const chatContent = document.createElement("div");
    chatContent.style.flex = "1";
    chatContent.style.display = "flex";
    chatContent.style.flexDirection = "column";
    chatContent.style.padding = "20px";
    chatContent.style.minHeight = "0";
    chatPanel.appendChild(chatContent);

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
      if (!file) return;

      // Check suffix
      const allowed = [".sqlite", ".sqlite3", ".csv", ".txt", ".json", ".py", ".js", ".c", ".cpp", ".java"];
      const name = file.name.toLowerCase();
      const validSuffix = allowed.some(suffix => name.endsWith(suffix));
      if (!validSuffix) {
        window.alert("Invalid file type. Allowed: .sqlite, .sqlite3, .csv, .txt, .json");
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
        // console.log("File content:", e.target?.result);
        // Add to uploadedFiles with unique id
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

    actionRow.appendChild(fileUploadBtn);
    actionRow.appendChild(fileInput);
    chatContent.appendChild(actionRow);

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
    clearBtn.textContent = "Clear";
    clearBtn.className = "btn btn-secondary";
    clearBtn.style.marginLeft = "4px";

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
      sendPostGPT(messages).then((gptResponse) => {
        botDiv.innerHTML = "<b>Daisen Bot:</b> " + convertMarkdownToHTML(autoWrapMath(gptResponse));
        messages.push({ role: "assistant", content: gptResponse });
        messagesDiv.scrollTop = messagesDiv.scrollHeight;
        console.log("GPT response:", gptResponse);

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

    document.body.appendChild(chatPanel);

    // Animate in
    setTimeout(() => {
      chatPanel.classList.add('open');
      // Hide the chat button
      this._showChatButton = false;
      this._addPaginationControl();
    }, 200);
  }

  _addPageButtons(ul: HTMLUListElement) {
    const numPages = Math.ceil(
      this._filteredNames.length / (this._numRow * this._numCol)
    );

    if (numPages === 0) {
      this._showNoComponentInfo();
      return;
    }

    this._addPrevPageButton(ul, this._currPage);

    this._addNumPageButton(ul, 0);

    if (this._currPage > 2) {
      this._addEllipsis(ul);
    }

    for (let i = Math.max(1, this._currPage - 1); i <= Math.min(numPages - 2, this._currPage + 1); i++) {
      this._addNumPageButton(ul, i);
    }

    if (this._currPage < numPages - 3) {
      this._addEllipsis(ul);
    }

    if (numPages > 1) {
      this._addNumPageButton(ul, numPages - 1);
    }

    this._addNextPageButton(ul, this._currPage, numPages);
    // let offset = -2;
    // if (this._currPage <= 1) {
    //   offset = -this._currPage;
    // }
    // if (this._currPage == numPages - 2) {
    //   offset = -3;
    // }
    // if (this._currPage == numPages - 1) {
    //   offset = -4;
    // }

    // for (let i = 0; i < 5; i++) {
    //   const pageNum = this._currPage + i + offset;
    //   if (pageNum < 0 || pageNum >= numPages) {
    //     continue;
    //   }

    //   this._addNumPageButton(ul, pageNum);
    // }

    // this._addNextPageButton(ul, this._currPage, numPages);
  }

  _addEllipsis(ul: HTMLUListElement) {
    const li = document.createElement("li");
    li.classList.add("page-item", "disabled");
    li.innerHTML = `<a class="page-link">...</a>`;
    ul.appendChild(li);
  }

  _showNoComponentInfo() {
    const div = document.createElement("div");
    div.classList.add("alert");
    div.classList.add("alert-warning");
    div.innerHTML = "No component selected.";

    this._pageBtnContainer.innerHTML = "";
    this._pageBtnContainer.appendChild(div);
  }

  _addPrevPageButton(ul: HTMLUListElement, currPageNum: number) {
    const li = document.createElement("li");
    li.classList.add("page-item");
    if (currPageNum == 0) {
      li.classList.add("disabled");
    }
    li.innerHTML = `
            <a class="page-link" aria-label="Previous">
                <span aria-hidden="true">&laquo;</span>
                <span class="sr-only">Previous</span>
            </a>
        `;

    if (currPageNum > 0) {
      li.onclick = () => {
        this._switchToPage(currPageNum - 1);
      };
    }

    ul.appendChild(li);
  }

  _addNextPageButton(
    ul: HTMLUListElement,
    currPageNum: number,
    totalNumPages: number
  ) {
    const li = document.createElement("li");
    li.classList.add("page-item");
    if (currPageNum == totalNumPages - 1) {
      li.classList.add("disabled");
    }
    li.innerHTML = `
            <a class="page-link" aria-label="Next">
                <span aria-hidden="true">&raquo;</span>
                <span class="sr-only">Next</span
            </a>
        `;

    if (currPageNum < totalNumPages - 1) {
      li.onclick = () => {
        this._switchToPage(currPageNum + 1);
      };
    }

    ul.appendChild(li);
  }

  _addNumPageButton(ul: HTMLUListElement, pageNum: number) {
    const li = document.createElement("li");
    li.classList.add("page-item");
    if (pageNum == this._currPage) {
      li.classList.add("active");
    }
    li.innerHTML = `
            <a class="page-link">
                ${pageNum + 1}
            </a>
        `;

    li.onclick = () => {
      this._switchToPage(pageNum);
    };

    ul.appendChild(li);
  }

  _switchToPage(pageNum: number) {
    this._currPage = pageNum;

    const params = new URLSearchParams(window.location.search);
    params.set("page", pageNum.toString());
    window.history.pushState(null, null, `/dashboard?${params.toString()}`);

    this._addPaginationControl();
    this._renderPage();
    const pageInfo = this._pageBtnContainer.querySelector('.page-info') as HTMLDivElement;
  }

  _renderPage() {
    this._canvas.innerHTML = "";
    const startIndex = this._currPage * this._numRow * this._numCol;
    const endIndex = startIndex + this._numRow * this._numCol;
    const componentsToShow = this._filteredNames.slice(startIndex, endIndex);

    this._widgets = [];
    componentsToShow.forEach((name) => {
      const widget = new Widget(name, this._canvas, this);
      this._widgets.push(widget);

      widget.createWidget(this._widgetWidth(), this._widgetHeight());

      widget.setXAxis(this._startTime, this._endTime);
      widget.setFirstAxis(this._primaryAxis);
      widget.setSecondAxis(this._secondaryAxis);

      widget.render(true);
    });
    this._addPaginationControl();
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
      console.log("Uploaded files:", this._uploadedFiles.map(f => ({ id: f.id, name: f.name })));
    };
    fileRow.appendChild(removeBtn);

    this._fileListRow.appendChild(fileRow);
  });
  this._fileListRow.style.marginBottom = "4px";
  // Log current file list with ids after every render
  console.log("Uploaded files:", this._uploadedFiles.map(f => ({ id: f.id, name: f.name })));
}

function formatFileSize(size: number): string {
  if (size < 1024) return `${size.toFixed(1)} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  return `${(size / (1024 * 1024)).toFixed(1)} MB`;
}

export default Dashboard;