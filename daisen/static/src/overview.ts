import { WidgetBase } from './widgets/widget-base';
import { SimulationInfoWidget } from './widgets/simulation-info-widget';
import { TracesWidget } from './widgets/traces-widget';
import { TaskAnalysisComponentWidget } from './widgets/task-analysis-component-widget';
import { TaskAnalysisTimeWidget } from './widgets/task-analysis-time-widget';
import { BlockingAnalysisTimeWidget } from './widgets/blocking-analysis-time-widget';
import { BlockingAnalysisComponentWidget } from './widgets/blocking-analysis-component-widget';
import { DashboardPreviewWidget } from './widgets/dashboard-preview-widget';
import { ChatPanel } from './chatpanel';

class Overview extends ChatPanel {
  _canvas: HTMLDivElement;
  _toolBar: HTMLFormElement;
  _widgets: WidgetBase[] = [];
  _burgerMenu: HTMLDivElement;
  _dropdownCanvas: HTMLDivElement;
  _showChatButton: boolean = true;
  _originalCanvasWidth: string = "";
  _handleResize: () => void;

  constructor() {
    super();
  }

  setCanvas(canvas: HTMLDivElement, toolBar: HTMLFormElement) {
    this._canvas = canvas;
    this._toolBar = toolBar;
    
    this._canvas.classList.add('overview-container');
    
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
      this._resizeWidgets();
    });

    this._updateNavbarVisibility();
  }

  protected _onChatPanelOpen() {
    this._showChatButton = false;
    this._addChatButton();

    const canvasContainer = this._canvas;
    if (canvasContainer) {
      this._originalCanvasWidth = canvasContainer.style.width;
      canvasContainer.style.transition = "width 0.3s cubic-bezier(.4,0,.2,1)";
      canvasContainer.style.width = "calc(100% - 600px)";
      this._getChatPanelWidth();
      setTimeout(() => {
        this._resizeWidgets();
      }, 300);
    }

    this._handleResize = () => {
      const innerContainer = document.getElementById("inner-container");
      if (innerContainer) {
        const rect = innerContainer.getBoundingClientRect();
        this._chatPanel.style.top = rect.top + "px";
        this._chatPanel.style.height = rect.height + "px";
      } else {
        this._chatPanel.style.top = "0";
        this._chatPanel.style.height = "100vh";
      }
      
      if (canvasContainer) {
        canvasContainer.style.width = "calc(100% - 600px)";
      }
      this._resizeWidgets();
    }

    window.addEventListener("resize", this._handleResize);
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

  _resizeWidgets() {
    this._widgets.forEach(widget => {
      widget.resize();
    });
  }

  render() {
    this._canvas.innerHTML = '';
    
    const widgetsGrid = document.createElement('div');
    widgetsGrid.classList.add('widgets-grid');
    this._canvas.appendChild(widgetsGrid);

    this._createWidgets(widgetsGrid);
    this._addChatButton();
  }

  private _createWidgets(container: HTMLDivElement) {
    const widgetConfigs = [
      {
        id: 'simulation-info',
        title: 'Simulation Information',
        widget: SimulationInfoWidget,
        size: 'medium' as const
      },
      {
        id: 'traces',
        title: 'Traces',
        widget: TracesWidget,
        size: 'medium' as const
      },
      {
        id: 'task-analysis-component',
        title: 'Task Analysis by Component',
        widget: TaskAnalysisComponentWidget,
        size: 'medium' as const
      },
      {
        id: 'task-analysis-time',
        title: 'Task Analysis by Time',
        widget: TaskAnalysisTimeWidget,
        size: 'medium' as const
      },
      {
        id: 'blocking-analysis-time',
        title: 'Blocking Analysis by Time',
        widget: BlockingAnalysisTimeWidget,
        size: 'medium' as const
      },
      {
        id: 'blocking-analysis-component',
        title: 'Blocking Analysis by Component',
        widget: BlockingAnalysisComponentWidget,
        size: 'medium' as const
      },
      {
        id: 'metrics-analysis',
        title: 'Metrics Analysis',
        widget: DashboardPreviewWidget,
        size: 'large' as const
      }
    ];

    this._widgets = [];

    widgetConfigs.forEach(config => {
      const widgetContainer = document.createElement('div');
      widgetContainer.classList.add('widget-container', `widget-${config.size}`);
      widgetContainer.id = config.id;
      container.appendChild(widgetContainer);

      const widgetInstance = new config.widget(widgetContainer, {
        title: config.title,
        size: config.size
      });

      this._widgets.push(widgetInstance);
      widgetInstance.render();
    });
  }

  private _addChatButton() {
    const existingButton = this._canvas.querySelector('.chat-button-container');
    if (existingButton) {
      existingButton.remove();
    }

    const chatButtonContainer = document.createElement('div');
    chatButtonContainer.classList.add('chat-button-container');
    
    const chatButton = document.createElement('button');
    chatButton.classList.add('btn', 'btn-secondary', 'overview-chat-button');
    chatButton.style.display = this._showChatButton ? "flex" : "none";
    chatButton.style.alignItems = "center";
    chatButton.style.backgroundColor = "#0d6efd";
    chatButton.style.borderColor = "#0d6efd";
    chatButton.innerHTML = `
      <span style="display:inline-block;width:30px;height:30px;margin-right:8px;">
        <svg width="30" height="30" viewBox="0 0 60 60" xmlns="http://www.w3.org/2000/svg">
          <path d="M30,12 L29,13 L29,15 L28,16 L28,17 L27,18 L27,20 L26,21 L26,22 L24,24 L23,24 L22,25 L21,25 L20,26 L19,26 L18,27 L16,27 L15,28 L14,28 L13,29 L13,30 L14,31 L16,31 L17,32 L18,32 L19,33 L21,33 L22,34 L23,34 L25,36 L25,37 L26,38 L26,39 L27,40 L27,41 L28,42 L28,44 L29,45 L29,46 L30,47 L31,47 L32,46 L32,44 L33,43 L33,42 L34,41 L34,39 L35,38 L35,37 L37,35 L38,35 L39,34 L40,34 L41,33 L42,33 L43,32 L45,32 L46,31 L47,31 L48,30 L48,29 L47,28 L45,28 L44,27 L43,27 L42,26 L40,26 L39,25 L38,25 L36,23 L36,22 L35,21 L35,20 L34,19 L34,18 L33,17 L33,15 L32,14 L32,13 L31,12 Z"
                fill="none" stroke="white" stroke-width="2" />
          <line x1="44" y1="10" x2="44" y2="18" stroke="white" stroke-width="2"/>
          <line x1="40" y1="14" x2="48" y2="14" stroke="white" stroke-width="2"/>
          <line x1="16" y1="42" x2="16" y2="50" stroke="white" stroke-width="2"/>
          <line x1="12" y1="46" x2="20" y2="46" stroke="white" stroke-width="2"/>
        </svg>
      </span>
      Daisen Bot
    `;

    chatButton.onclick = async () => {
      this._showChatPanel();
      
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
        this._chatPanel.classList.remove('open');
        this._chatPanel.classList.add('closing');
        setTimeout(() => {
          this._chatPanel.remove();
          window.removeEventListener("resize", this._handleResize);
          this._showChatButton = true;
          this._chatPanelWidth = 0;
          this._addChatButton();
          
          if (this._canvas) {
            this._canvas.style.width = this._originalCanvasWidth;
            setTimeout(() => {
              this._resizeWidgets();
            }, 200);
          }
        }, 200);
      };
      
      this._chatPanel.appendChild(closeBtn);
    };

    chatButtonContainer.appendChild(chatButton);
    this._canvas.appendChild(chatButtonContainer);
  }
}

export default Overview;