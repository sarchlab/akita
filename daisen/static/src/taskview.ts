import * as d3 from "d3";
import TaskYIndexAssigner from "./taskyindexassigner";
import TaskRenderer from "./taskrenderer";
import XAxisDrawer from "./xaxisdrawer";
import { Task } from "./task";

class TaskView {
  private _yIndexAssigner: TaskYIndexAssigner;
  private _taskRenderer: TaskRenderer;
  private _xAxisDrawer: XAxisDrawer;
  private _canvas: HTMLElement;
  private _tooltip: HTMLElement;
  private _canvasWidth: number;
  private _canvasHeight: number;
  private _marginTop: number;
  private _marginBottom: number;
  private _marginLeft: number;
  private _marginRight: number;
  private _startTime: number;
  private _endTime: number;
  private _xScale: d3.ScaleLinear<number, number>;
  private _task: Task;
  private _parentTask: Task;
  private _subTasks: Array<Task>;
  private _allTasks: Array<Task>;
  private _maxY: number;
  private _largeTaskHeight: number;
  private _taskGroupGap: number;

  constructor(
    yIndexAssigner: TaskYIndexAssigner,
    taskRenderer: TaskRenderer,
    xAxisDrawer: XAxisDrawer
  ) {
    this._yIndexAssigner = yIndexAssigner;
    this._taskRenderer = taskRenderer;
    this._xAxisDrawer = xAxisDrawer;

    this._canvas = null;
    this._tooltip = null;
    this._canvasWidth = 0;
    this._canvasHeight = 0;
    this._marginTop = 20;
    this._marginBottom = 20;
    this._marginLeft = 5;
    this._marginRight = 5;
    this._largeTaskHeight = 15;
    this._taskGroupGap = 10;

    this._startTime = 0;
    this._endTime = 0;
    this._xScale = null;
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

    this.updateLayout();
    this._doRender();
  }

  updateLayout() {
    this._canvasWidth = this._canvas.offsetWidth;
    this._canvasHeight = this._canvas.offsetHeight;
    d3.select(this._canvas)
      .select("svg")
      .attr("width", this._canvasWidth.toString())
      .attr("height", this._canvasHeight.toString());
    this._updateTimeScale();
    this._drawDivider();
  }

  private _drawDivider() {
    const svg = d3.select(this._canvas).select("svg");
    let dividerGroup = svg.select(".divider");
    dividerGroup.remove();
    dividerGroup = svg.append("g").attr("class", "divider");

    dividerGroup
      .append("text")
      .attr("font-size", 20)
      .attr("fout-weight", "bold")
      .attr("x", 5)
      .attr("y", this._marginTop + this._taskGroupGap + 15)
      .attr("text-anchor", "left")
      .attr(
        "style",
        "text-shadow: -1px -1px 0 #ffffff, 1px -1px 0 #ffffff, -1px 1px 0 #ffffff, 1px 1px 0 #ffffff"
      )
      .text("Parent Task");

    dividerGroup
      .append("text")
      .attr("font-size", 20)
      .attr("fout-weight", "bold")
      .attr("x", 5)
      .attr(
        "y",
        this._marginTop + this._taskGroupGap * 2 + this._largeTaskHeight + 16
      )
      .attr("text-anchor", "left")
      .attr(
        "style",
        "text-shadow: -1px -1px 0 #ffffff, 1px -1px 0 #ffffff, -1px 1px 0 #ffffff, 1px 1px 0 #ffffff"
      )
      .text("Current Task");

    dividerGroup
      .append("text")
      .attr("font-size", 20)
      .attr("fout-weight", "bold")
      .attr("x", 5)
      .attr(
        "y",
        this._marginTop +
          this._taskGroupGap * 3 +
          this._largeTaskHeight * 2 +
          16
      )
      .attr("text-anchor", "left")
      //   .attr("stroke", "#ffffff")
      .attr(
        "style",
        "text-shadow: -1px -1px 0 #ffffff, 1px -1px 0 #ffffff, -1px 1px 0 #ffffff, 1px 1px 0 #ffffff; pointer-events:none; "
      )
      .text("Subtasks");

    const divider1Y =
      this._marginTop + this._taskGroupGap * 1.5 + this._largeTaskHeight;
    dividerGroup
      .append("line")
      .attr("x1", 0)
      .attr("x2", this._canvasWidth)
      .attr("y1", divider1Y)
      .attr("y2", divider1Y)
      .attr("stroke", "#000000")
      .attr("stroke-dasharray", 4);

    const divider2Y =
      this._marginTop + this._taskGroupGap * 2.5 + this._largeTaskHeight * 2;
    dividerGroup
      .append("line")
      .attr("x1", 0)
      .attr("x2", this._canvasWidth)
      .attr("y1", divider2Y)
      .attr("y2", divider2Y)
      .attr("stroke", "#000000")
      .attr("stroke-dasharray", 4);
  }

  setTimeAxis(startTime: number, endTime: number) {
    this._startTime = startTime;
    this._endTime = endTime;
    this._xAxisDrawer.setTimeRange(startTime, endTime);
    this._updateTimeScale();
  }

  private _updateTimeScale() {
    this._xScale = d3
      .scaleLinear()
      .domain([this._startTime, this._endTime])
      .range([this._marginLeft, this._canvasWidth - this._marginLeft]);

    this._taskRenderer.setXScale(this._xScale);
    this._drawXAxis();
  }

  private _drawXAxis() {
    this._xAxisDrawer
    .setCanvasHeight(this._canvasHeight)
    .setCanvasWidth(this._canvasWidth)
    .setScale(this._xScale)
    .renderCustom(5); 
  }

  updateXAxis() {
    this._taskRenderer.updateXAxis();
  }

  highlight(task: Task | ((t: Task) => boolean)) {
    this._taskRenderer.hightlight(task);
  }

  render(task: Task, subTasks: Array<Task>, parentTask: Task) {
    this._task = task;
    this._subTasks = subTasks;
    this._parentTask = parentTask;

    let tasks = [];
    if (parentTask != null) {
      parentTask.isParentTask = true;
      tasks.push(parentTask);
    }
    task.isMainTask = true;
    tasks.push(task);
    tasks = tasks.concat(subTasks);
    this._allTasks = tasks;
    this._maxY = this._yIndexAssigner.assign(subTasks);

    this._doRender();
  }

  _doRender() {
    if (!this._allTasks) {
      return;
    }

    const tasks = this._allTasks;
    const barRegionHeight =
      this._canvasHeight - this._marginBottom - this._marginTop;
    const nonSubTaskRegionHeight =
      this._taskGroupGap * 4 + this._largeTaskHeight * 2;
    const subTaskRegionHeight = barRegionHeight - nonSubTaskRegionHeight;
    let barHeight = subTaskRegionHeight / this._maxY;
    if (barHeight > 10) {
      barHeight = 10;
    }

    this._taskRenderer
      .renderWithHeight((task: Task) => {
        if (task.isParentTask) {
          return this._largeTaskHeight;
        } else if (task.isMainTask) {
          return this._largeTaskHeight;
        } else {
          return barHeight * 0.75;
        }
      })
      .renderWithY((task: Task) => {
        if (task.isParentTask) {
          let extraHeight = this._taskGroupGap;
          return extraHeight + this._marginTop;
        } else if (task.isMainTask) {
          let extraHeight = this._taskGroupGap * 2 + this._largeTaskHeight;
          return extraHeight + this._marginTop;
        } else {
          let extraHeight = this._taskGroupGap * 3 + this._largeTaskHeight * 2;
          return task.yIndex * barHeight + extraHeight + this._marginTop;
        }
      })
      .render(tasks);
  }
}

export default TaskView;
