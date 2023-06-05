import * as d3 from "d3";
import TaskYIndexAssigner from "./taskyindexassigner";
import TaskRenderer from "./taskrenderer";
import XAxisDrawer from "./xaxisdrawer";
import { ScaleLinear } from "d3";
import { Task, Dim } from "./task";

class ComponentView {
  _yIndexAssigner: TaskYIndexAssigner;
  _taskRenderer: TaskRenderer;
  _xAxisDrawer: XAxisDrawer;
  _canvas: HTMLElement;
  _canvasWidth: number;
  _canvasHeight: number;
  _marginTop: number;
  _marginBottom: number;
  _marginLeft: number;
  _marginRight: number;
  _startTime: number;
  _endTime: number;
  _xScale: ScaleLinear<number, number>;
  _tasks: Array<Task>;

  constructor(
    yIndexAssigner: TaskYIndexAssigner,
    taskRenderer: TaskRenderer,
    xAxisDrawer: XAxisDrawer
  ) {
    this._yIndexAssigner = yIndexAssigner;
    this._taskRenderer = taskRenderer;
    this._xAxisDrawer = xAxisDrawer;

    this._marginTop = 5;
    this._marginLeft = 5;
    this._marginRight = 5;
    this._marginBottom = 20;
  }

  setCanvas(canvas: HTMLElement, tooltip: HTMLElement) {
    this._canvas = canvas;
    this._canvasWidth = this._canvas.offsetWidth;
    this._canvasHeight = this._canvas.offsetHeight;

    this._taskRenderer.setCanvas(canvas, tooltip);
    this._xAxisDrawer.setCanvas(canvas);

    d3.select(this._canvas)
      .select("svg")
      .attr("width", this._canvasWidth)
      .attr("height", this._canvasHeight);

    this._updateTimeScale();
    this._renderData();
  }

  updateLayout() {
    this._canvasWidth = this._canvas.offsetWidth;
    this._canvasHeight = this._canvas.offsetHeight;
    d3.select(this._canvas)
      .select("svg")
      .attr("width", this._canvasWidth)
      .attr("height", this._canvasHeight);
    this._updateTimeScale();
  }

  setTimeAxis(startTime: number, endTime: number) {
    this._startTime = startTime;
    this._endTime = endTime;
    this._updateTimeScale();
  }

  _updateTimeScale() {
    this._xScale = d3
      .scaleLinear()
      .domain([this._startTime, this._endTime])
      .range([0, this._canvasWidth]);
    this._taskRenderer.setXScale(this._xScale);

    this._drawXAxis();
  }

  _drawXAxis() {
    this._xAxisDrawer
      .setCanvasHeight(this._canvasHeight)
      .setCanvasWidth(this._canvasWidth)
      .setScale(this._xScale)
      .renderTop(-15)
      .renderBottom();
  }

  updateXAxis() {
    this._taskRenderer.updateXAxis();
  }

  highlight(task: Task | ((t: Task) => boolean)) {
    this._taskRenderer.hightlight(task);
  }

  render(tasks: Array<Task>) {
    this._tasks = tasks;

    this._renderData();

    if (tasks.length > 0) {
      this._showLocation(tasks[0]);
    } else {
      this._removeLocation();
    }
  }

  _renderData() {
    if (!this._tasks) {
      return;
    }

    let tasks = this._tasks;
    const tree = this.rearrangeAsTree(<Array<Task>>tasks);
    const initDim = new Dim();
    initDim.x = 0;
    initDim.y = this._marginTop;
    initDim.width = this._canvasWidth;
    initDim.height = this._canvasHeight - this._marginTop - this._marginBottom;
    initDim.startTime = this._startTime;
    initDim.endTime = this._endTime;
    this.assignDimension(tree, initDim);

    // this._normalizeTaskHeight(tasks);

    tasks.sort((a: Task, b: Task) => a.level - b.level);
    tasks = this._filterTasks(tasks);

    this._taskRenderer
      .renderWithX((t: Task) => t.dim.x)
      .renderWithY((t: Task) => t.dim.y)
      .renderWithHeight((t: Task) => t.dim.height)
      .renderWithWidth((t: Task) => t.dim.width)
      .render(tasks);
  }

  private rearrangeAsTree(tasks: Array<Task>): Task {
    const virtualRoot = new Task();
    virtualRoot.level = 0;

    const forest = new Array<Task>();
    const idToTaskMap = {};

    tasks.forEach(task => {
      task.subTasks = [];
      idToTaskMap[task.id] = task;
    });

    tasks.forEach(task => {
      const parentTask = idToTaskMap[task.parent_id];
      if (!parentTask) {
        forest.push(task);
      } else {
        parentTask.subTasks.push(task);
      }
    });

    forest.forEach(t => {
      this.assignTaskLevel(t, 1);
    });

    virtualRoot.subTasks = forest;

    return virtualRoot;
  }

  private assignTaskLevel(task: Task, level: number) {
    task.level = level;
    task.subTasks.forEach(t => {
      this.assignTaskLevel(t, level + 1);
    });
  }

  private assignDimension(tree: Task, containerDim: Dim) {
    let taskHeight = containerDim.height;
    let depth = 0;
    tree.dim = containerDim;
    while (taskHeight > 0) {
      taskHeight = this.assignDimensionLevel(tree, taskHeight, depth);
      depth++;
    }
  }

  private assignDimensionLevel(
    tree: Task,
    parentLevelHeight: number,
    depth: number
  ): number {
    if (parentLevelHeight < 2) {
      return 0;
    }

    const tasksOfLevel = new Array<Task>();
    this.getTasksAtLevel(tree, depth, tasksOfLevel);

    if (tasksOfLevel.length === 0) {
      return 0;
    }

    let globalMaxY = -1;
    tasksOfLevel.forEach(t => {
      const maxY = this._yIndexAssigner.assign(t.subTasks);
      if (maxY > globalMaxY) {
        globalMaxY = maxY;
      }
    });

    if (globalMaxY == -1) {
      return 0;
    }

    const taskHeight = parentLevelHeight / (globalMaxY + 1);
    const paddedTaskHeight = this.padTaskHeight(taskHeight);

    tasksOfLevel.forEach(t => {
      const paddedDim = Object.assign({}, t.dim);
      // paddedDim.height -= 6;
      // paddedDim.y += 3;

      const subTasks = t.subTasks;
      subTasks.forEach(subT => {
        const dim = new Dim();
        dim.startTime = subT.start_time;
        dim.endTime = subT.end_time;

        // dim.height = paddedDim.height / (globalMaxY + 1);
        dim.height = paddedTaskHeight;
        dim.y = taskHeight * subT.yIndex + paddedDim.y;
        dim.y += (taskHeight - paddedTaskHeight) / 2;
        // if (dim.height > 10) {
        //   dim.y += dim.height * 0.02;
        // } else {
        //   dim.y += dim.height * 0.01;
        // }

        const pixelPerSecond = t.dim.width / (t.dim.endTime - t.dim.startTime);
        const duration = subT.end_time - subT.start_time;
        const offsetDuration = subT.start_time - paddedDim.startTime;
        dim.width = duration * pixelPerSecond;
        dim.x = offsetDuration * pixelPerSecond + paddedDim.x;

        subT.dim = dim;
      });
    });

    return paddedTaskHeight;
  }

  private getTasksAtLevel(tree: Task, depth: number, returnTasks: Array<Task>) {
    if (depth === 0) {
      returnTasks.push(tree);
      return;
    }

    tree.subTasks.forEach(t => {
      this.getTasksAtLevel(t, depth - 1, returnTasks);
    });
  }

  private padTaskHeight(height: number): number {
    if (height > 10) {
      return height * 0.8;
    } else {
      return height * 0.6;
    }
  }

  _filterTasks(tasks: Array<Task>) {
    return tasks.filter(t => {
      if (t.level == 1) {
        return true;
      }

      if (!t.dim) {
        return false;
      }

      if (t.dim.width < 1) {
        return false;
      }

      if (t.dim.height < 1) {
        return false;
      }

      return true;
    });
  }

  _showLocation(task: Task) {
    const svg = d3.select(this._canvas).select("svg");
    let locationLabel = svg.select(".location-label");
    if (locationLabel.empty()) {
      locationLabel = svg
        .append("text")
        .attr("x", 5)
        .attr("y", 40 + this._marginTop)
        .attr("class", "location-label")
        .attr(
          "style",
          "font-size: 3.2em; opacity: 0.4; color: #000000; pointer-events: none; text-shadow: -1px -1px 0 #ffffff, 1px -1px 0 #ffffff, -1px 1px 0 #ffffff,1px 1px 0 #ffffff;"
        );
    }

    locationLabel.text(task["where"]);
  }

  _removeLocation() {
    const svg = d3.select(this._canvas).select("svg");
    const locationLabel = svg.select(".location-label");
    if (locationLabel.empty()) {
      return;
    }
    locationLabel.text("");
  }
}

export default ComponentView;
