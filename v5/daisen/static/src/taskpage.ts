import TaskView from "./taskview";
import { TaskColorCoder } from "./taskcolorcoder";
import Legend from "./taskcolorlegend";
import ComponentView from "./componentview";
import TaskYIndexAssigner from "./taskyindexassigner";
import TaskRenderer from "./taskrenderer";
import XAxisDrawer from "./xaxisdrawer";
import { ZoomHandler } from "./mouseeventhandler";
import { Task } from "./task";
import { smartString } from "./smartvalue";
import { Widget, TimeValue } from "./widget";
import Dashboard from "./dashboard";

import { ChatPanel } from "./chatpanel";

export class TaskPage extends ChatPanel implements ZoomHandler {
  _container: HTMLElement;
  _taskViewCanvas: HTMLElement;
  _componentViewCanvas: HTMLElement;
  _leftColumn: HTMLElement;
  _rightColumn: HTMLElement;
  _leftColumnWidth: number;
  _rightColumnWidth: number;
  _tooltip: HTMLElement;
  _legendCanvas: HTMLElement;
  _componentOnlyMode: boolean;
  _componentName: string;
  _dissectionMode: boolean;
  _currTasks: Object;
  _startTime: number;
  _endTime: number;
  _taskColorCoder: TaskColorCoder;
  _legend: Legend;
  _yIndexAssigner: TaskYIndexAssigner;
  _taskView: TaskView;
  _componentView: ComponentView;
  _widget: Widget;
  _showChatButton: boolean = true; // Add this flag to control the right chat button visibility
  _originalCanvasWidth: string = ""; // Store the original width of the canvas before shrinking
  _handleResize: () => void;


  constructor() {
    super();
    this._container = null;
    this._taskViewCanvas = null;
    this._componentViewCanvas = null;
    this._leftColumn = null;
    this._rightColumn = null;
    this._rightColumnWidth = 350;
    this._leftColumnWidth = 0;
    this._tooltip = null;
    this._legendCanvas = null;
    this._componentOnlyMode = false;
    this._componentName = "";
    this._dissectionMode = false;
    this._widget = null;

    this._currTasks = {
      task: null,
      subTask: null,
      parentTasks: [],
      sameLocationTasks: []
    };

    this._startTime = 0;
    this._endTime = 0;

    this._taskColorCoder = new TaskColorCoder();
    this._legend = new Legend(this._taskColorCoder, this);
    this._yIndexAssigner = new TaskYIndexAssigner();
    const widgetCanvas = document.createElement('div');
    document.body.appendChild(widgetCanvas);      
    this._widget = new Widget(this._componentName, widgetCanvas, new Dashboard());
    this._taskView = new TaskView(
      this._yIndexAssigner,
      new TaskRenderer(this, this._taskColorCoder),
      new XAxisDrawer()
    );
    this._taskView.setToggleCallback(() => {
      if (this._componentOnlyMode) {
      } else {
        this._toggleDissectionMode();
      }
    });
    this._componentView = new ComponentView(
      this._yIndexAssigner,
      new TaskRenderer(this, this._taskColorCoder),
      new XAxisDrawer(),
      this._widget
    );
    this._componentView.setComponentName(this._componentName);
    this._componentView.setPrimaryAxis('ReqInCount');
    this._componentView.setTimeAxis(this._startTime, this._endTime);
    
    this._initializeURLNavigation();
  }

  protected _setTraceComponentNames() {
    this._traceStartTime = this._startTime;
    this._traceEndTime = this._endTime;
    this._traceAllComponentNames = [this._componentName];
    this._traceCurrentComponentNames = [this._componentName];
  }

  _handleMouseMove(e: MouseEvent) {
    document
      .getElementById("mouse-x-coordinate")
      .setAttribute("transform", `translate(${e.clientX}, 0)`);
    document
      .getElementById("mouse-y-coordinate")
      .setAttribute("transform", `translate(0, ${e.clientY})`);

    const duration = this._endTime - this._startTime;
    const pixels = this._leftColumn.clientWidth;
    const timePerPixel = duration / pixels;
    const pixelOnLeft = e.clientX - this._leftColumn.clientLeft;
    const timeOnLeft = timePerPixel * pixelOnLeft;
    const currTime = timeOnLeft + this._startTime;
    document.getElementById("mouse-time").innerHTML = smartString(currTime);
  }

  getAxisStatus(): [number, number, number, number] {
    return [this._startTime, this._endTime, 0, this._leftColumnWidth];
  }

  temporaryTimeShift(startTime: number, endTime: number): void {
    this.setTimeRange(startTime, endTime, false);
  }

  permanentTimeShift(startTime: number, endTime: number): void {
    this.setTimeRange(startTime, endTime, true);
  }

  domElement(): HTMLElement | SVGElement {
    return this._leftColumn;
  }

  setTimeRange(startTime: number, endTime: number, reload = false) {
    this._startTime = startTime;
    this._endTime = endTime;
    this._taskView.setTimeAxis(this._startTime, this._endTime);
    this._componentView.setTimeAxis(this._startTime, this._endTime);

    if (!reload) {
      this._taskView.updateXAxis();
      this._componentView.updateXAxis();
      return;
    }

    const params = new URLSearchParams(window.location.search);
    params.set("starttime", startTime.toString());
    params.set("endtime", endTime.toString());
    window.history.replaceState(
      null,
      null,
      `${window.location.pathname}?${params.toString()}`
    );

    if (this._componentOnlyMode) {
      this.showComponent(this._componentName);
    } else {
      this.showTask(this._currTasks["task"], true);
    }
  }

  layout() {
    this._container = document.getElementById("inner-container");
    const containerHeight = window.innerHeight - 76;
    this._container.style.height = containerHeight.toString() + "px";

    this._addChatButtons();
    this._layoutLeftColumn();
    this._layoutRightColumn();

    this._taskView.setCanvas(this._taskViewCanvas, this._tooltip);
    this._componentView.setCanvas(this._componentViewCanvas, this._tooltip);
    this._legend.setCanvas(this._legendCanvas);
    this._updateLayout();
  }

  _layoutRightColumn() {
    if (this._rightColumn === null) {
      this._rightColumn = document.createElement("div");
      this._rightColumn.classList.add("column");
      this._rightColumn.classList.add("side-column");
      this._container.appendChild(this._rightColumn);
    }
    const locationLabel = document.createElement("div");
    locationLabel.setAttribute("id", "location-label");
    locationLabel.style.fontSize = "20px";
    locationLabel.style.color = "black"; 
    locationLabel.style.fontWeight = "bold"; 
    this._rightColumn.appendChild(locationLabel);
    this._rightColumn.style.width =
      this._rightColumnWidth.toString() + "px";
    this._rightColumn.style.height =
      this._container.offsetHeight.toString() + "px";
    // const marginLeft = -5;
    // this._rightColumn.style.marginLeft = marginLeft.toString();

    if (this._tooltip === null) {
      this._tooltip = document.createElement("div");
      this._tooltip.classList.add("curr-task-info");
      this._rightColumn.appendChild(this._tooltip);
    }

    if (this._legendCanvas === null) {
      this._legendCanvas = document.createElement("div");
      this._legendCanvas.innerHTML = "<svg></svg>";
      this._legendCanvas.classList.add("legend");
      this._rightColumn.appendChild(this._legendCanvas);
    }
  }

  _layoutLeftColumn() {
    if (this._leftColumn === null) {
      this._leftColumn = document.createElement("div");
      this._leftColumn.classList.add("column");
      this._container.appendChild(this._leftColumn);
      this._leftColumn.addEventListener("mousemove", e => {
        this._handleMouseMove(e);
      });
    }
    this._leftColumnWidth =
      this._container.offsetWidth - this._rightColumnWidth - 1;
    this._leftColumn.style.width = this._leftColumnWidth.toString() + "px";
    const height = this._container.offsetHeight;
    this._leftColumn.style.height = height.toString() + "px";

    this._layoutTaskView();
    this._layoutComponentView();
  }

  _layoutComponentView() {
    if (this._componentViewCanvas === null) {
      this._componentViewCanvas = document.createElement("div");
      this._componentViewCanvas.setAttribute("id", "component-view");
      this._componentViewCanvas.appendChild(
        document.createElementNS("http://www.w3.org/2000/svg", "svg")
      );
      this._leftColumn.appendChild(this._componentViewCanvas);
    }
    this._componentViewCanvas.style.width =
      this._leftColumn.offsetWidth.toString() + "px";
    const height = this._leftColumn.offsetHeight - 200;
    this._componentViewCanvas.style.height = height.toString() + "px";
  }

  _layoutTaskView() {
    if (this._taskViewCanvas === null) {
      this._taskViewCanvas = document.createElement("div");
      this._taskViewCanvas.setAttribute("id", "task-view");
      this._taskViewCanvas.appendChild(
        document.createElementNS("http://www.w3.org/2000/svg", "svg")
      );

      this._leftColumn.appendChild(this._taskViewCanvas);
    }
    this._taskViewCanvas.style.width =
      this._leftColumn.offsetWidth.toString() + "px";
    this._taskViewCanvas.style.height = "200px";
    // (this._leftColumn.offsetHeight / 2);
  }

  showTaskWithId(id: string) {
    const task = new Task();
    task.id = id;
    this.showTask(task);
  }

  highlight(task: Task | ((t: Task) => boolean)) {
    this._taskView.highlight(task);
    this._componentView.highlight(task);
  }

  async showTask(task: Task, keepView = false) {
    this._switchToTaskMode();

    let rsps = await Promise.all([
      fetch(`/api/trace?id=${task.id}`),
      fetch(`/api/trace?parentid=${task.id}`)
    ]);

    task = await rsps[0].json();
    task = task[0];
    const subTasks = await rsps[1].json();

    if (!keepView) {
      this._updateTimeAxisAccordingtoTask(task);
      this._taskView.updateLayout();
      this._componentView.updateLayout();
    }

    let parentTask = null;
    if (task.parent_id != "") {
      parentTask = await fetch(`/api/trace?id=${task.parent_id}`).then(rsp =>
        rsp.json()
      );
    }
    if (parentTask != null && parentTask.length > 0) {
      parentTask = parentTask[0];
    }

    if (!keepView) {
      this._updateTimeAxisAccordingtoTask(task);
      this._taskView.updateLayout();
      this._componentView.updateLayout();
    }
    if (parentTask != null) {
      this._componentView.setComponentName(parentTask.location);
    } else {
      this._componentView.setComponentName(task.location);
    }

    const traceRsps = await Promise.all([
      fetch(
        `/api/trace?` +
        `where=${task.location}` +
        `&starttime=${this._startTime}` +
        `&endtime=${this._endTime}`
      )
    ]);
    const sameLocationTasks = await traceRsps[0].json();

    this._currTasks["task"] = task;
    this._currTasks["parentTask"] = parentTask;
    this._currTasks["subTasks"] = subTasks;
    this._currTasks["sameLocationTasks"] = sameLocationTasks;

    let tasks = new Array(task);
    if (parentTask != null) {
      parentTask.isParentTask = true;
      tasks.push(parentTask);
    }
    task.isMainTask = true;
    tasks.push(task);
    tasks = tasks.concat(subTasks);
    tasks.forEach(t => {
      if (t.start_time === undefined) {
        t.start_time = 0;
      }
    });

    this._taskColorCoder.recode(tasks.concat(sameLocationTasks));
    this._taskView.render(task, subTasks, parentTask);
    this._componentView.render(sameLocationTasks);
    this._legend.render();
  }

  async showComponent(name: string) {
    this._componentName = name;
    this._componentView.setComponentName(name);
    console.log('TaskPage', this._componentName);
    this._switchToComponentOnlyMode();
    await this._waitForComponentNameUpdate();
    const rsps = await Promise.all([
      fetch(
        `/api/trace?` +
        `where=${name}` +
        `&starttime=${this._startTime}` +
        `&endtime=${this._endTime}`
      )
    ]);
    const sameLocationTasks = await rsps[0].json();

    this._taskColorCoder.recode(sameLocationTasks);
    this._legend.render();
    console.log('ComponentView Component Name before render:', 
      this._componentView._componentName);

    await this._componentView.render(sameLocationTasks);
  }

  _waitForComponentNameUpdate() {
    return new Promise((resolve) => {
        setTimeout(() => {
            resolve(true);
        }, 1000);
    });
  }

  _switchToComponentOnlyMode() {
    this._componentOnlyMode = true;

    // this._taskViewCanvas.style.height = 0
    // this._componentViewCanvas.style.height =
    //     this._leftColumn.offsetHeight
  }

  _switchToTaskMode() {
    this._componentOnlyMode = false;

    // this._taskViewCanvas.style.height = 200
    // this._componentViewCanvas.style.height =
    //     this._leftColumn.offsetHeight - 200
  }

  _toggleDissectionMode() {
    this._dissectionMode = !this._dissectionMode;
    this._updateURLAndLayout();
  }


  _initializeURLNavigation() {
    const urlParams = new URLSearchParams(window.location.search);
    const isDissectMode = urlParams.get('dissect') === '1';
    
    if (isDissectMode) {
      this._dissectionMode = true;
      this._updateLayout();
    }
    
    window.addEventListener('popstate', () => {
      this._handleURLChange();
    });
  }

  _handleURLChange() {
    const urlParams = new URLSearchParams(window.location.search);
    const shouldBeDissectMode = urlParams.get('dissect') === '1';
    
    if (shouldBeDissectMode !== this._dissectionMode) {
      this._dissectionMode = shouldBeDissectMode;
      this._updateLayout();
    }
  }

  _updateURLAndLayout() {
    const url = new URL(window.location.href);
    if (this._dissectionMode) {
      url.searchParams.set('dissect', '1');
    } else {
      url.searchParams.delete('dissect');
    }
    
    window.history.pushState({}, '', url.toString());
    
    this._updateLayout();
  }

  _updateLayout() {
    if (this._dissectionMode) {
      // In dissection mode: just show dissection view overlay, don't change any layout
      this._taskView.showDissectionView();
    } else {
      this._taskView.hideDissectionView();
    }
    
    this._taskView.updateLayout();
    if (!this._dissectionMode) {
      this._componentView.updateLayout();
    }
  }

  _updateTimeAxisAccordingtoTask(task: Task) {
    const duration = task.end_time - task.start_time;
    this._startTime = task.start_time - 0.2 * duration;
    this._endTime = task.end_time + 0.2 * duration;
    this._taskView.setTimeAxis(this._startTime, this._endTime);
    this._componentView.setTimeAxis(this._startTime, this._endTime);
  }

  // async sendChatMessage(message: string) {
  //   this._chatMessages.push({ role: "user", content: message });

  //   const response = await sendPostGPT(this._chatMessages);

  //   const botMessage = response.choices[0].message.content;
  //   this._chatMessages.push({ role: "assistant", content: botMessage });

  //   this._renderChatMessage("user", message);
  //   this._renderChatMessage("assistant", botMessage);
  // }

  // _renderChatMessage(role: "user" | "assistant", content: string) {
  //   const chatPanel = document.getElementById("chat-panel");

  //   const messageElement = document.createElement("div");
  //   messageElement.classList.add("chat-message");
  //   messageElement.classList.add(role);

  //   const contentElement = document.createElement("div");
  //   contentElement.classList.add("chat-content");
  //   contentElement.innerHTML = content;

  //   messageElement.appendChild(contentElement);
  //   chatPanel.appendChild(messageElement);

  //   // Scroll to the bottom of the chat panel
  //   chatPanel.scrollTop = chatPanel.scrollHeight;
  // }

  // toggleChatPanel() {
  //   const chatPanel = document.getElementById("chat-panel");
  //   chatPanel.style.display =
  //     chatPanel.style.display === "none" || chatPanel.style.display === ""
  //       ? "block"
  //       : "none";
  // }

  // clearChat() {
  //   this._chatMessages = [];
  //   const chatPanel = document.getElementById("chat-panel");
  //   chatPanel.innerHTML = "";
  // }

  // renderMath(expression: string, elementId: string) {
  //   const element = document.getElementById(elementId);
  //   if (element) {
  //     element.innerHTML = ""; // Clear previous content
  //     katex.render(expression, element, {
  //       throwOnError: false
  //     });
  //   }
  // }

  protected _onChatPanelOpen() {
    // this._resize();
    // this._renderPage();
    // this._addPaginationControl();
    this._showChatButton = false;

    // Store the original width before shrinking
    const canvasContainer = this._container;
    // let originalCanvasWidth = "";
    if (canvasContainer) {
      this._originalCanvasWidth = canvasContainer.style.width;
      canvasContainer.style.transition = "width 0.3s cubic-bezier(.4,0,.2,1)";
      canvasContainer.style.width = "calc(100% - 600px)";
      this._getChatPanelWidth();
      setTimeout(() => {
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
    }

    window.addEventListener("resize", this._handleResize);
  }

  _addChatButtons() {
    const topNav = document.getElementById("top-nav");
    if (!topNav) return;

    // Remove existing chat button if present
    const existingChatBtn = topNav.querySelector(".daisen-chat-btn");
    if (existingChatBtn) {
      existingChatBtn.remove();
    }

    const chatButton = document.createElement("button");
    chatButton.classList.add("btn", "btn-secondary", "ml-3", "daisen-chat-btn");
    chatButton.style.display = "flex";
    chatButton.style.alignItems = "center";
    chatButton.style.paddingRight = "20px";
    chatButton.style.marginRight = "15px";
    chatButton.style.backgroundColor = "#0d6efd";
    chatButton.style.borderColor = "#0d6efd";
    chatButton.style.marginLeft = "auto";

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
    const container = document.getElementById("container");
    chatButton.onclick = () => {
      this._showChatButton = false;
      chatButton.style.visibility = this._showChatButton ? "visible" : "hidden";
      this._originalCanvasWidth = container.style.width;
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
          // this._addPaginationControl();
          // Restore the canvas container width to its original value
          if (this._container) {
            this._container.style.width = this._originalCanvasWidth;// || "100%";
            setTimeout(() => {
              // this.layout();
              this._showChatButton = true;
              chatButton.style.visibility = this._showChatButton ? "visible" : "hidden";
            }, 0);
          }
        }, 200); // Match the CSS transition duration
      };
      this._chatPanel.appendChild(closeBtn);
    };

    topNav.appendChild(chatButton);
  }



    //   closeBtn.onclick = () => {
    //   // Animate out
    //   chatPanel.classList.remove('open');
    //   chatPanel.classList.add('closing');
    //   setTimeout(() => {
    //     chatPanel.remove();
    //     window.removeEventListener("resize", handleResize);
    //     // Restore the canvas container width to its original value
    //     if (canvasContainer) {
    //       canvasContainer.style.width = originalCanvasWidth || "100%";
    //       setTimeout(() => {
    //         this.layout();
    //       }, 300);
    //     }
    //   }, 200); // Match the CSS transition duration
    // };

  // _injectChatPanelCSS() {
  //   if (document.getElementById('chat-panel-anim-style')) return;
  //   const style = document.createElement('style');
  //   style.id = 'chat-panel-anim-style';
  //   style.innerHTML = `
  //     #chat-panel {
  //       transition: transform 0.3s cubic-bezier(.4,0,.2,1), opacity 0.3s cubic-bezier(.4,0,.2,1);
  //       transform: translateX(100%);
  //       opacity: 0;
  //     }
  //     #chat-panel.open {
  //       transform: translateX(0);
  //       opacity: 1;
  //     }
  //     #chat-panel.closing {
  //       transform: translateX(100%);
  //       opacity: 0;
  //     }
  //   `;
  //   document.head.appendChild(style);
  // }

  // _showChatPanel() {
  //   let messages = this._chatMessages;
  //   this._injectChatPanelCSS();

  //   // Remove existing panel if any
  //   let oldPanel = document.getElementById("chat-panel");
  //   if (oldPanel) oldPanel.remove();

  //   // Create the chat panel
  //   const chatPanel = document.createElement("div");
  //   chatPanel.id = "chat-panel";
  //   chatPanel.style.position = "fixed";
  //   chatPanel.style.right = "0";
  //   chatPanel.style.width = "600px";
  //   chatPanel.style.background = "rgba(255,255,255,0.7)";
  //   chatPanel.style.zIndex = "9999";
  //   chatPanel.style.boxShadow = "0 0 10px rgba(0,0,0,0.2)";
  //   chatPanel.style.display = "flex";
  //   chatPanel.style.flexDirection = "column";
  //   chatPanel.style.justifyContent = "flex-start";
  //   chatPanel.style.overflow = "hidden";

  //   // Set chat panel height and top to match #inner-container
  //   const innerContainer = document.getElementById("inner-container");
  //   if (innerContainer) {
  //     const rect = innerContainer.getBoundingClientRect();
  //     chatPanel.style.top = rect.top + "px";
  //     chatPanel.style.height = rect.height + "px";
  //   } else {
  //     // fallback to full viewport height if not found
  //     chatPanel.style.top = "0";
  //     chatPanel.style.height = "100vh";
  //   }

  //   // Store the original width before shrinking
  //   const canvasContainer = this._container;
  //   let originalCanvasWidth = "";
  //   if (canvasContainer) {
  //     originalCanvasWidth = canvasContainer.style.width;
  //     canvasContainer.style.transition = "width 0.3s cubic-bezier(.4,0,.2,1)";
  //     canvasContainer.style.width = "calc(100% - 600px)";
  //     setTimeout(() => {
  //       this.layout();
  //     }, 300);
  //   }

  //   // Triangle close button
  //   const closeBtn = document.createElement("button");
  //   closeBtn.style.position = "absolute";
  //   closeBtn.style.left = "-4px";
  //   closeBtn.style.top = "50%";
  //   closeBtn.style.transform = "translateY(-50%)";
  //   closeBtn.style.width = "12px";
  //   closeBtn.style.height = "40px";
  //   closeBtn.style.background = "transparent";
  //   closeBtn.style.border = "none";
  //   closeBtn.style.cursor = "pointer";
  //   closeBtn.title = "Close";

  //   closeBtn.innerHTML = `
  //     <svg width="12" height="40" viewBox="0 0 12 40" xmlns="http://www.w3.org/2000/svg">
  //       <defs>
  //         <linearGradient id="silverGradient" x1="0" y1="0" x2="1" y2="0">
  //           <stop offset="0%" stop-color="#d9d9d9"/>
  //           <stop offset="50%" stop-color="#b0b0b0"/>
  //           <stop offset="100%" stop-color="#f5f5f5"/>
  //         </linearGradient>
  //       </defs>
  //       <polygon points="0,0 12,20 0,40" fill="url(#silverGradient)" stroke="#aaa" stroke-width="0.5"/>
  //     </svg>
  //   `;

  //   const handleResize = () => {
  //     //Adjust chat panel height and top
  //     const innerContainer = document.getElementById("inner-container");
  //     if (innerContainer) {
  //       const rect = innerContainer.getBoundingClientRect();
  //       chatPanel.style.top = rect.top + "px";
  //       chatPanel.style.height = rect.height + "px";
  //     } else {
  //       chatPanel.style.top = "0";
  //       chatPanel.style.height = "100vh";
  //     }
  //     // Shrink canvas again (in case window size changed)
  //     if (canvasContainer) {
  //       canvasContainer.style.width = "calc(100% - 600px)";
  //     }
  //     // Re-render
  //     this.layout();
  //   }

  //   window.addEventListener("resize", handleResize);

  //   closeBtn.onclick = () => {
  //     // Animate out
  //     chatPanel.classList.remove('open');
  //     chatPanel.classList.add('closing');
  //     setTimeout(() => {
  //       chatPanel.remove();
  //       window.removeEventListener("resize", handleResize);
  //       // Restore the canvas container width to its original value
  //       if (canvasContainer) {
  //         canvasContainer.style.width = originalCanvasWidth || "100%";
  //         setTimeout(() => {
  //           this.layout();
  //         }, 300);
  //       }
  //     }, 200); // Match the CSS transition duration
  //   };

  //   chatPanel.appendChild(closeBtn);

  //   const chatContent = document.createElement("div");
  //   chatContent.style.flex = "1";
  //   chatContent.style.display = "flex";
  //   chatContent.style.flexDirection = "column";
  //   chatContent.style.padding = "20px";
  //   chatContent.style.minHeight = "0";
  //   chatPanel.appendChild(chatContent);

  //   // Message display area
  //   const messagesDiv = document.createElement("div");
  //   messagesDiv.style.flex = "1 1 0%";
  //   messagesDiv.style.height = "0";
  //   messagesDiv.style.overflowY = "auto";
  //   messagesDiv.style.marginBottom = "10px";
  //   messagesDiv.style.background = "rgba(255, 255, 255, 0.5)";
  //   messagesDiv.style.borderRadius = "6px";
  //   messagesDiv.style.padding = "8px";
  //   chatContent.appendChild(messagesDiv);

  //   // Loading messages
  //   messages
  //     .filter(m => m.role !== "system")
  //     .forEach(m => {
  //       if (m.role === "user") {
  //         const userDiv = document.createElement("div");
  //         userDiv.style.display = "flex";
  //         userDiv.style.justifyContent = "flex-end";
  //         userDiv.style.margin = "4px 0";

  //         const userBubble = document.createElement("span");
  //         userBubble.innerHTML = "<b>You:</b> " + m.content;
  //         userBubble.style.background = "#0d6efd";
  //         userBubble.style.color = "white";
  //         userBubble.style.padding = "8px 12px";
  //         userBubble.style.borderRadius = "16px";
  //         userBubble.style.maxWidth = "90%";
  //         userBubble.style.display = "inline-block";
  //         userBubble.style.wordBreak = "break-word";
  //         userDiv.appendChild(userBubble);

  //         messagesDiv.appendChild(userDiv);
  //       } else if (m.role === "assistant") {
  //         const botDiv = document.createElement("div");
  //         botDiv.innerHTML = "<b>Daisen Bot:</b> " + this._convertMarkdownToHTML(this._autoWrapMath(m.content));
  //         botDiv.style.textAlign = "left";
  //         botDiv.style.margin = "4px 0";
  //         messagesDiv.appendChild(botDiv);
  //       }
  //     });

  //   // apply KaTeX rendering for math
  //   messagesDiv.querySelectorAll('.math').forEach(el => {
  //     try {
  //       const tex = el.textContent || "";
  //       const displayMode = el.getAttribute("data-display") === "block";
  //       console.log("Rendering math:", tex, "Display mode:", displayMode);
  //       el.innerHTML = katex.renderToString(tex, { displayMode });
  //     } catch (e) {
  //       el.innerHTML = "<span style='color:red'>Invalid math</span>";
  //       console.log("KaTeX error:", e, "for tex:", el.textContent);
  //     }
  //   });

  //   const historyMenu = document.createElement("div");
  //   historyMenu.style.display = "flex";
  //   historyMenu.style.flexDirection = "column";
  //   historyMenu.style.marginBottom = "8px";
  //   chatContent.appendChild(historyMenu);

  //   function renderHistoryMenu() {
  //     const lastUserMessages = messages.filter(m => m.role === "user").slice(-3);
  //     historyMenu.innerHTML = "";
  //     lastUserMessages.forEach(msg => {
  //       const item = document.createElement("button");
  //       // Limit to 10 words for display
  //       const words = msg.content.split(" ");
  //       let displayText = msg.content;
  //       if (words.length > 10) {
  //         displayText = words.slice(0, 10).join(" ") + "...";
  //       }
  //       item.textContent = displayText;
  //       item.style.background = "#f8f9fa";
  //       item.style.border = "none";
  //       item.style.borderRadius = "16px";
  //       item.style.padding = "10px 16px";
  //       item.style.margin = "4px 0";
  //       item.style.fontSize = "1em";
  //       item.style.color = "#222";
  //       item.style.boxShadow = "0 2px 8px rgba(0,0,0,0.06)";
  //       item.style.cursor = "pointer";
  //       item.style.transition = "background 0.15s, box-shadow 0.15s";
  //       // Hover effect
  //       item.onmouseenter = () => {
  //         item.style.background = "#e9ecef";
  //       };
  //       item.onmouseleave = () => {
  //         item.style.background = "#f8f9fa";
  //       }; 

  //       // Fills input on click
  //       item.onclick = () => {
  //         input.value = msg.content;
  //         input.focus();
  //       };
  //       historyMenu.appendChild(item);
  //     });
  //   }

  //   // When panel opens
  //   renderHistoryMenu();

  //   // Initial welcome message
  //   const welcomeDiv = document.createElement("div");
  //   welcomeDiv.innerHTML = "<b>Daisen Bot:</b> Hello! What can I help you with today?";
  //   welcomeDiv.style.textAlign = "left";
  //   welcomeDiv.style.marginBottom = "8px";
  //   messagesDiv.appendChild(welcomeDiv);

  //   // Input area
  //   const inputContainer = document.createElement("div");
  //   inputContainer.style.display = "flex";
  //   inputContainer.style.gap = "8px";

  //   const input = document.createElement("textarea");
  //   input.placeholder = "Type a message...";
  //   input.rows = 1;
  //   input.style.flex = "1";
  //   input.style.padding = "6px";
  //   input.style.borderRadius = "4px";
  //   input.style.border = "1px solid #ccc";
  //   input.style.resize = "none";
  //   input.style.overflowY = "auto";
  //   input.style.minHeight = "38px";
  //   input.style.maxHeight = "130px";

  //   // Auto-resize as user types
  //   input.addEventListener("input", function() {
  //     this.style.height = "auto";
  //     this.style.height = (this.scrollHeight) + "px";
  //   });

  //   const sendBtn = document.createElement("button");
  //   sendBtn.textContent = "Send";
  //   sendBtn.className = "btn btn-primary";

  //   const clearBtn = document.createElement("button");
  //   clearBtn.textContent = "Clear";
  //   clearBtn.className = "btn btn-secondary";
  //   clearBtn.style.marginLeft = "4px";

  //   // Send handler
  //   const sendMessage = () => {
  //     const userMsg = input.value.trim();
  //     if (!userMsg) return;

  //     // Disable send button while waiting
  //     sendBtn.disabled = true;
  //     input.disabled = true;

  //     // User message
  //     const userDiv = document.createElement("div");
  //     userDiv.style.display = "flex";
  //     userDiv.style.justifyContent = "flex-end";
  //     userDiv.style.margin = "4px 0";

  //     const userBubble = document.createElement("span");
  //     userBubble.innerHTML = "<b>You:</b> " + userMsg;
  //     userBubble.style.background = "#0d6efd";
  //     userBubble.style.color = "white";
  //     userBubble.style.padding = "8px 12px";
  //     userBubble.style.borderRadius = "16px";
  //     userBubble.style.maxWidth = "90%";
  //     userBubble.style.display = "inline-block";
  //     userBubble.style.wordBreak = "break-word";
  //     userDiv.appendChild(userBubble);

  //     messagesDiv.appendChild(userDiv);

  //     // Call GPT with full history
  //     messages.push({ role: "user", content: userMsg });

  //     // Show history menu
  //     renderHistoryMenu();
      
  //     // Clear input field
  //     input.value = "";

  //     // Show "thinking message"
  //     const botDiv = document.createElement("div");
  //     botDiv.innerHTML = "<b>Daisen Bot:</b> <i>Thinking...</i>";;
  //     botDiv.style.textAlign = "left";
  //     botDiv.style.margin = "4px 0";
  //     messagesDiv.appendChild(botDiv);

  //     // Call GPT and update the message
  //     sendPostGPT(messages).then((gptResponse) => {
  //       botDiv.innerHTML = "<b>Daisen Bot:</b> " + this._convertMarkdownToHTML(this._autoWrapMath(gptResponse));
  //       messages.push({ role: "assistant", content: gptResponse });
  //       messagesDiv.scrollTop = messagesDiv.scrollHeight;
  //       console.log("GPT response:", gptResponse);

  //       // Apply KaTeX rendering for math in the new messages
  //       botDiv.querySelectorAll('.math').forEach(el => {
  //         try {
  //           const tex = el.textContent || "";
  //           const displayMode = el.getAttribute("data-display") === "block";
  //           console.log("Rendering math:", tex, "Display mode:", displayMode);
  //           el.innerHTML = katex.renderToString(tex, { displayMode });
  //         } catch (e) {
  //           el.innerHTML = "<span style='color:red'>Invalid math</span>";
  //           console.log("KaTeX error:", e, "for tex:", el.textContent);
  //         }
  //       });
        
  //       // Re-enable send button
  //       sendBtn.disabled = false;
  //       input.disabled = false;
  //       input.focus();
  //     });
  //     this._chatMessages = messages; // Update chat messages in the class
  //   };

  //   sendBtn.onclick = sendMessage;
  //   input.addEventListener("keydown", (e) => {
  //     if (e.key === "Enter" && !e.shiftKey) {
  //       e.preventDefault();
  //       sendMessage();
  //     }
  //   });

  //   clearBtn.onclick = () => {
  //     messages.length = 0;
  //     messages.push({ role: "system", content: "You are Daisen Bot." });
  //     input.value = "";
  //     // Remove all messages from the chat panel except the welcome message
  //     messagesDiv.innerHTML = "";
  //     const welcomeDiv = document.createElement("div");
  //     welcomeDiv.innerHTML = "<b>Daisen Bot:</b> Hello! What can I help you with today?";
  //     welcomeDiv.style.textAlign = "left";
  //     welcomeDiv.style.marginBottom = "8px";
  //     messagesDiv.appendChild(welcomeDiv);
  //     renderHistoryMenu();
  //     input.style.height = "38px";
  //   };

  //   inputContainer.appendChild(input);
  //   inputContainer.appendChild(sendBtn);
  //   inputContainer.appendChild(clearBtn);
  //   chatContent.appendChild(inputContainer);

  //   document.body.appendChild(chatPanel);

  //   // Animate in
  //   setTimeout(() => {
  //     chatPanel.classList.add('open');
  //   }, 200);
  // }

  // _convertMarkdownToHTML(text: string): string {
  //   // Basic markdown conversion (you can enhance this)
  //   return text
  //     .replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
  //     .replace(/\*(.*?)\*/g, '<em>$1</em>')
  //     .replace(/`([^`]+)`/g, '<code>$1</code>')
  //     .replace(/\n/g, '<br>');
  // }

  // _autoWrapMath(text: string): string {
  //   // Wrap math expressions with proper HTML for KaTeX rendering
  //   return text
  //     .replace(/\$\$([^$]+)\$\$/g, '<span class="math" data-display="block">$1</span>')
  //     .replace(/\$([^$]+)\$/g, '<span class="math" data-display="inline">$1</span>');
  // }



  _smartString(value: number): string {
    if (value < 0.001) {
      return (value * 1000000).toFixed(2) + 'μs';
    } else if (value < 1) {
      return (value * 1000).toFixed(2) + 'ms';
    } else {
      return value.toFixed(2) + 's';
    }
  }
}

export default TaskPage;