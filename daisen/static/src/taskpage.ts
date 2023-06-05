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
  _currTasks: Object;
  _startTime: number;
  _endTime: number;
  _taskColorCoder: TaskColorCoder;
  _legend: Legend;
  _yIndexAssigner: TaskYIndexAssigner;
  _taskView: TaskView;
  _componentView: ComponentView;

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
    this._taskView = new TaskView(
      this._yIndexAssigner,
      new TaskRenderer(this, this._taskColorCoder),
      new XAxisDrawer()
    );
    this._componentView = new ComponentView(
      this._yIndexAssigner,
      new TaskRenderer(this, this._taskColorCoder),
      new XAxisDrawer()
    );
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
    this._container.style.height = containerHeight.toString();

    this._layoutLeftColumn();
    this._layoutRightColumn();

    this._taskView.setCanvas(this._taskViewCanvas, this._tooltip);
    this._componentView.setCanvas(this._componentViewCanvas, this._tooltip);
    this._legend.setCanvas(this._legendCanvas);
  }

  _layoutRightColumn() {
    if (this._rightColumn === null) {
      this._rightColumn = document.createElement("div");
      this._rightColumn.classList.add("column");
      this._rightColumn.classList.add("side-column");
      this._container.appendChild(this._rightColumn);
    }
    this._rightColumn.style.width = this._rightColumnWidth.toString();
    this._rightColumn.style.height = this._container.offsetHeight.toString();
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
    this._leftColumn.style.width = this._leftColumnWidth.toString();
    const height = this._container.offsetHeight;
    this._leftColumn.style.height = height.toString();

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
    this._componentViewCanvas.style.width = this._leftColumn.offsetWidth.toString();
    const height = this._leftColumn.offsetHeight - 200;
    this._componentViewCanvas.style.height = height.toString();
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
    this._taskViewCanvas.style.width = this._leftColumn.offsetWidth.toString();
    this._taskViewCanvas.style.height = "200";
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
      this._updateTimeAxisAccordingToTask(task);
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

    const traceRsps = await Promise.all([
      fetch(
        `/api/trace?` +
          `where=${task.where}` +
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
    this._switchToComponentOnlyMode();

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

    this._componentView.render(sameLocationTasks);
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

  _updateTimeAxisAccordingToTask(task: Task) {
    const duration = task.end_time - task.start_time;
    this._startTime = task.start_time - 0.2 * duration;
    this._endTime = task.end_time + 0.2 * duration;
    this._taskView.setTimeAxis(this._startTime, this._endTime);
    this._componentView.setTimeAxis(this._startTime, this._endTime);
  }
}
