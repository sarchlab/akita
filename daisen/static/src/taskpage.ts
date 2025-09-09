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

export class TaskPage implements ZoomHandler {
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
  _componentMilestoneMode: boolean;
  _currTasks: Object;
  _startTime: number;
  _endTime: number;
  _taskColorCoder: TaskColorCoder;
  _legend: Legend;
  _yIndexAssigner: TaskYIndexAssigner;
  _taskView: TaskView;
  _componentView: ComponentView;
  _widget: Widget;
  constructor() {
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
    this._componentMilestoneMode = false;
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
        this._toggleComponentMilestoneMode();
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
      this._updateTimeAxisAccordingToTask(task);
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

  _toggleComponentMilestoneMode() {
    if (!this._componentOnlyMode) return;
    this._componentMilestoneMode = !this._componentMilestoneMode;
    this._updateLayout();
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
      this._hideComponentMilestoneView();
    } else if (this._componentMilestoneMode && this._componentOnlyMode) {
      // In component milestone mode: don't change any layout, just show milestone view overlay
      this._taskView.hideDissectionView();
      this._showComponentMilestoneView();
    } else {
      this._taskView.hideDissectionView();
      this._hideComponentMilestoneView();
    }
    
    this._taskView.updateLayout();
    if (!this._dissectionMode && !this._componentMilestoneMode) {
      this._componentView.updateLayout();
    }
  }

  _updateTimeAxisAccordingToTask(task: Task) {
    const duration = task.end_time - task.start_time;
    this._startTime = task.start_time - 0.2 * duration;
    this._endTime = task.end_time + 0.2 * duration;
    this._taskView.setTimeAxis(this._startTime, this._endTime);
    this._componentView.setTimeAxis(this._startTime, this._endTime);
  }

  _showComponentMilestoneView() {
    this._createComponentMilestoneView();
  }

  _hideComponentMilestoneView() {
    // Check body for milestone view since it's fixed positioned
    const milestoneView = document.body.querySelector('.component-milestone-view');
    if (milestoneView) {
      milestoneView.remove();
    }
  }

  async _createComponentMilestoneView() {
    // Fetch all tasks for this component
    const response = await fetch(
      `/api/trace?where=${this._componentName}&starttime=${this._startTime}&endtime=${this._endTime}`
    );
    const componentTasks = await response.json();

    // Remove existing view
    this._hideComponentMilestoneView();

    // Create milestone view container - same positioning as TaskView dissection view
    const milestoneView = document.createElement('div');
    milestoneView.className = 'component-milestone-view';
    const leftColumnWidth = this._leftColumn.offsetWidth;
    
    milestoneView.style.cssText = `
      position: fixed;
      top: ${this._leftColumn.getBoundingClientRect().top + 200}px;
      left: ${this._leftColumn.getBoundingClientRect().left}px;
      width: ${leftColumnWidth}px;
      height: ${this._leftColumn.offsetHeight - 200}px;
      background: white;
      padding: 20px;
      overflow-y: auto;
      border-top: 2px solid #ccc;
      z-index: 1000;
    `;

    // Create title
    const title = document.createElement('h2');
    title.textContent = `Component Milestones: ${this._componentName}`;
    title.style.cssText = `
      font-size: 20px;
      font-weight: bold;
      margin-bottom: 20px;
      color: #333;
    `;
    milestoneView.appendChild(title);

    // Collect all milestones from all tasks
    const allMilestones = [];
    componentTasks.forEach(task => {
      if (task.steps && task.steps.length > 0) {
        task.steps.forEach(step => {
          allMilestones.push({
            ...step,
            taskId: task.id,
            taskKind: task.kind,
            taskWhat: task.what
          });
        });
      }
    });

    if (allMilestones.length === 0) {
      const noMilestones = document.createElement('div');
      noMilestones.textContent = 'No milestones found for this component.';
      noMilestones.style.cssText = `
        font-size: 16px;
        color: #666;
        text-align: center;
        margin-top: 50px;
      `;
      milestoneView.appendChild(noMilestones);
      this._leftColumn.appendChild(milestoneView);
      return;
    }

    // Group milestones by kind for swimlanes
    const milestonesByKind = {};
    allMilestones.forEach(milestone => {
      const kind = milestone.kind || 'unknown';
      if (!milestonesByKind[kind]) {
        milestonesByKind[kind] = [];
      }
      milestonesByKind[kind].push(milestone);
    });

    // Sort milestones within each kind by time
    Object.keys(milestonesByKind).forEach(kind => {
      milestonesByKind[kind].sort((a, b) => a.time - b.time);
    });

    // Create swimlane chart
    const swimlaneContainer = document.createElement('div');
    swimlaneContainer.style.cssText = `
      position: relative;
      width: 100%;
      min-height: 400px;
      border: 1px solid #ddd;
      border-radius: 8px;
      background: #f9f9f9;
    `;

    const containerWidth = this._leftColumnWidth - 80;
    const timeRange = this._endTime - this._startTime;
    const laneHeight = 60;
    const kinds = Object.keys(milestonesByKind).sort();
    const colors = ['#FF6B6B', '#FFD93D', '#52C41A', '#9B59B6', '#FF8C00', '#1E90FF', '#20B2AA'];

    // Create each swimlane
    kinds.forEach((kind, kindIndex) => {
      const lane = document.createElement('div');
      lane.style.cssText = `
        position: relative;
        height: ${laneHeight}px;
        margin-bottom: 10px;
        border: 1px solid #ccc;
        border-radius: 4px;
        background: ${colors[kindIndex % colors.length]}20;
      `;

      // Lane label
      const label = document.createElement('div');
      label.textContent = kind;
      label.style.cssText = `
        position: absolute;
        left: 10px;
        top: 50%;
        transform: translateY(-50%);
        font-weight: bold;
        font-size: 14px;
        color: #333;
        z-index: 10;
        background: white;
        padding: 2px 8px;
        border-radius: 4px;
        border: 1px solid #ddd;
      `;
      lane.appendChild(label);

      // Add milestones for this kind
      milestonesByKind[kind].forEach((milestone) => {
        const milestoneX = 100 + ((milestone.time - this._startTime) / timeRange) * (containerWidth - 120);
        
        if (milestoneX >= 100 && milestoneX <= containerWidth) {
          const milestoneMarker = document.createElement('div');
          milestoneMarker.style.cssText = `
            position: absolute;
            left: ${milestoneX}px;
            top: 50%;
            transform: translate(-50%, -50%);
            width: 12px;
            height: 12px;
            background: ${colors[kindIndex % colors.length]};
            border: 2px solid white;
            border-radius: 50%;
            cursor: pointer;
            z-index: 5;
          `;

          // Add tooltip on hover
          milestoneMarker.addEventListener('mouseenter', (e) => {
            this._showComponentMilestoneTooltip(milestone, e);
          });

          milestoneMarker.addEventListener('mouseleave', () => {
            this._hideComponentMilestoneTooltip();
          });

          lane.appendChild(milestoneMarker);
        }
      });

      swimlaneContainer.appendChild(lane);
    });

    milestoneView.appendChild(swimlaneContainer);
    // Add to body since we're using fixed positioning
    document.body.appendChild(milestoneView);
  }

  _showComponentMilestoneTooltip(milestone, event) {
    const tooltip = this._tooltip;
    if (tooltip) {
      tooltip.innerHTML = `
        <div style="text-align: left; min-width: 250px;">
          <h4>Milestone at ${this._smartString(milestone.time)}</h4>
          <div style="margin-bottom: 8px;">
            <span style="background-color: #ffeb3b; padding: 2px 4px; border-radius: 3px;">Kind:</span> ${milestone.kind || 'N/A'}<br/>
            <span style="background-color: #e3f2fd; padding: 2px 4px; border-radius: 3px;">What:</span> ${milestone.what || 'N/A'}<br/>
            <span style="background-color: #f3e5f5; padding: 2px 4px; border-radius: 3px;">Task:</span> ${milestone.taskKind} - ${milestone.taskWhat}<br/>
            <span style="background-color: #e8f5e8; padding: 2px 4px; border-radius: 3px;">Task ID:</span> ${milestone.taskId}
          </div>
        </div>
      `;
      tooltip.classList.add('showing');
    }
  }

  _hideComponentMilestoneTooltip() {
    const tooltip = this._tooltip;
    if (tooltip) {
      tooltip.classList.remove('showing');
    }
  }

  _hideMilestoneKindsLegend() {
    const legend = document.querySelector('.milestone-kinds-legend');
    if (legend) {
      (legend as HTMLElement).style.display = 'none';
    }
  }

  _showMilestoneKindsLegend() {
    const legend = document.querySelector('.milestone-kinds-legend');
    if (legend) {
      (legend as HTMLElement).style.display = 'block';
    }
  }

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