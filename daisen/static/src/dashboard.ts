import * as d3 from "d3";
import Widget from "./widget";
import { thresholdFreedmanDiaconis } from "d3";
import { ChatPanel } from "./chatpanel";
import { sendGetCheckEnvFile } from "./chatpanelrequests";

class YAxisOption {
  optionValue: string;
  html: string;
}

class Dashboard extends ChatPanel {
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
  _originalCanvasWidth: string = ""; // Store the original width of the canvas before shrinking
  _handleResize: () => void;
  // _chatMessages: { role: "user" | "assistant" | "system"; content: string }[] = [
  //   { role: "system", content: "You are Daisen Bot." }
  // ];  // Make the message history global
  // _uploadedFiles: { id: number; name: string; content: string; size: string }[] = [];
  // _fileUploadBtn: HTMLButtonElement;
  // _fileIdCounter: number;
  // _fileListRow: HTMLDivElement; // Add this to hold the file list container

  // _attachRepoVisible: boolean = false;
  // _attachRepoChecks: { [key: string]: boolean } = {};
  // _githubIsAvailableResponse: { available: number; routine_keys: string[] } | null = null;


  constructor() {
    super();
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

  protected _onChatPanelOpen() {
    // this._resize();
    // this._renderPage();
    // this._addPaginationControl();
    this._showChatButton = false;
    this._addPaginationControl();

    // Store the original width before shrinking
    const canvasContainer = this._canvas;
    // let originalCanvasWidth = "";
    if (canvasContainer) {
      this._originalCanvasWidth = canvasContainer.style.width;
      canvasContainer.style.transition = "width 0.3s cubic-bezier(.4,0,.2,1)";
      canvasContainer.style.width = "calc(100% - 600px)";
      this._getChatPanelWidth();
      setTimeout(() => {
        this._resize();
        this._renderPage();
      }, 300);
    }

    this._handleResize = () => {
      //Adjust chat panel height and top
      const innerContainer = document.getElementById("inner-container");
      if (innerContainer) {
        const rect = innerContainer.getBoundingClientRect();
        this._chatPanel.style.top = rect.top + "px";
        this._chatPanel.style.height = rect.height + "px";
      } else {
        this._chatPanel.style.top = "0";
        this._chatPanel.style.height = "100vh";
      }
      // Shrink canvas again (in case window size changed)
      if (canvasContainer) {
        canvasContainer.style.width = "calc(100% - 600px)";
      }
      // Re-render widgets
      this._resize();
      this._renderPage();
    }

    window.addEventListener("resize", this._handleResize);
  }

  protected _setTraceComponentNames() {
    this._traceStartTime = this._startTime;
    this._traceEndTime = this._endTime;
    this._traceAllComponentNames = this._componentNames;
    this._traceCurrentComponentNames = this._filteredNames.slice(this._currPage * this._numRow * this._numCol, (this._currPage + 1) * this._numRow * this._numCol);
    // console.log("_traceCurrentComponentNames:", this._traceCurrentComponentNames.length, this._traceCurrentComponentNames);
    // console.log("this._numCol:", this._numCol, "this._numRow:", this._numRow);
    // console.log("_currPage:", this._currPage, "this._numWidget:", this._numWidget);
    // console.log("this._startTime:", this._startTime, "this._endTime:", this._endTime);
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
    if (width - this._chatPanelWidth >= 1500) { // if (width >= 1200) {
      this._numCol = 4;
    }
    if (width - this._chatPanelWidth < 1500 && width - this._chatPanelWidth >= 1000) { // if (width < 1200 && width >= 800) {
      this._numCol = 3;
    }
    if (width - this._chatPanelWidth < 1000) { // if (width < 800) {
      this._numCol = 2;
    }
    // console.log(width, height);
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
        console.log("simulation[0]:", simulation[0]);
        console.log("compNames:", compNames);
        simulation = simulation[0];

        // compNames.sort();

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
    chatButton.onclick = async () => {
      // Check if .env file exists before opening chat
      const envCheck = await sendGetCheckEnvFile();
      if (!envCheck.exists) {
        const userConfirms = confirm(
          'The .env file does not exist. This is required for DaisenBot to function properly.\n' +
          'Please create an .env file in the akita/daisen/ directory with your OpenAIAPI credentials.\n' +
          "Example:\n"+
          "```\n"+
          "OPENAI_URL=\"https://api.openai.com/v1/chat/completions\"\n"+
          "OPENAI_MODEL=\"gpt-4o\"\n"+
          "OPENAI_API_KEY=\"Bearer sk-proj-XXXXXXXXXXXX\"\n"+
          "GITHUB_PERSONAL_ACCESS_TOKEN=\"Bearer ghp_XXXXXXXXXXXX\"\n"+
          "```\n"+
          "Please refer to https://github.com/sarchlab/akita/tree/main/daisen#readme for more details.\n",
        );
        if (!userConfirms) {
          return; // Don't open chat if user cancels
        }
      }
      
      this._showChatPanel();

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
      closeBtn.style.zIndex = "10001";
      closeBtn.title = "Close";
      closeBtn.style.visibility = this._showChatButton ? "hidden": "visible";

  //     closeBtn.innerHTML = `
  // <svg width="14.4" height="48" viewBox="0 0 14.4 48" xmlns="http://www.w3.org/2000/svg">
  //   <!-- Blue triangle background, 20% larger -->
  //   <polygon points="0,0 14.4,24 0,48" fill="#0d6efd" stroke="#0d6efd" stroke-width="0.5"/>
  //   <!-- Centered white SVG path (example: a check mark) -->
  //   <path d="M4 24 L7 31 L11 17" stroke="#fff" stroke-width="2.5" fill="none" stroke-linecap="round" stroke-linejoin="round"/>
  //   <!-- You can replace the above path with any full SVG path starting with M and ending with Z -->
  // </svg>
  //     `;

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

      closeBtn.onclick = () => {
        // Animate out
        this._chatPanel.classList.remove('open');
        this._chatPanel.classList.add('closing');
        setTimeout(() => {
          this._chatPanel.remove();
          window.removeEventListener("resize", this._handleResize);
          this._showChatButton = true;
          this._chatPanelWidth = 0;
          console.log("❌ Chat Panel Closed - Width reset to 0px");
          this._addPaginationControl();
          // Restore the canvas container width to its original value
          if (this._canvas) {
            this._canvas.style.width = this._originalCanvasWidth;// || "100%";
            setTimeout(() => {
              this._resize();
              this._renderPage();
            }, 200);
          }
        }, 200); // Match the CSS transition duration
      };
      this._chatPanel.appendChild(closeBtn);
    };

    

    // this._chatPanel.appendChild(closeBtn);

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
    // paginationContainer.appendChild(closeBtn);

    if (this._canvas.querySelector('.pagination-container')) {
      this._canvas.removeChild(this._canvas.querySelector('.pagination-container'));
    }
    this._canvas.appendChild(paginationContainer);
    this._addPageButtons(ul);
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

export default Dashboard;