import * as d3 from "d3";
import TaskYIndexAssigner from "./taskyindexassigner";
import TaskRenderer from "./taskrenderer";
import XAxisDrawer from "./xaxisdrawer";
import { ScaleLinear } from "d3";
import { Task, Dim } from "./task";
import { Widget, TimeValue } from "./widget";

type DataObject = {
  info_type: string;
  data: TimeValue[];
};

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
  _widget: Widget;
  _graphContentWidth: number;
  _graphContentHeight: number;
  _graphWidth: number;
  _graphHeight: number;
  _titleHeight: number;
  _graphPaddingTop: number;
  _widgetHeight: number;
  _widgetWidth: number;
  _yAxisWidth: number;
  _xAxisHeight: number;
  _primaryAxis: string;
  _secondaryAxis: string;
  _numDots:number;
  _componentName: string;
  _svg: SVGElement;
  _yScale: d3.ScaleLinear<number, number>;
  _primaryYScale: d3.ScaleLinear<number, number>;
  _secondaryYScale: d3.ScaleLinear<number, number>;
  _primaryAxisData: object;
  _secondaryAxisData: object;

  constructor(
    yIndexAssigner: TaskYIndexAssigner,
    taskRenderer: TaskRenderer,
    xAxisDrawer: XAxisDrawer,
    widget: Widget
  ) {
    this._yIndexAssigner = yIndexAssigner;
    this._taskRenderer = taskRenderer;
    this._xAxisDrawer = xAxisDrawer;
    this._widget = widget; 

    this._marginTop = 5;
    this._marginLeft = 5;
    this._marginRight = 5;
    this._marginBottom = 20;

    this._numDots = 40;
    this._widgetHeight = 100;
    this._widgetWidth = 0;
    this._yAxisWidth = 55;
    this._graphWidth = this._widgetWidth;
    this._graphContentWidth = this._widgetWidth - 2 * this._yAxisWidth;
    this._titleHeight = 20;
    this._graphHeight = this._widgetHeight - this._titleHeight;
    this._graphPaddingTop = 5;
    this._xAxisHeight = 30;
    this._graphContentHeight =
    this._graphHeight - this._xAxisHeight - this._graphPaddingTop;
    this._componentName = this._widget._componentName;

    this._startTime = 0;
    this._endTime = 0;
    this._primaryAxis = "ConcurrentTask";
    this._secondaryAxis = "ConcurrentTask";
    this._xScale = null;
  }

  setComponentName(componentName: string) {
    this._componentName = componentName;
    console.log('Component Name set to:', this._componentName);
  }
  
  setCanvas(canvas: HTMLElement, tooltip: HTMLElement) {
    this._canvas = canvas;
    this._canvasWidth = this._canvas.offsetWidth;
    this._canvasHeight = this._canvas.offsetHeight;

    this._taskRenderer.setCanvas(canvas, tooltip);
    this._xAxisDrawer.setCanvas(canvas);

    const svg = d3.select(this._canvas)
      .select<SVGSVGElement>("svg")
      .attr("width", this._canvasWidth)
      .attr("height", this._canvasHeight);
    
    const svgElement = svg.node() as SVGElement;

    this._updateTimeScale();
    this._renderData();

    this._fetchAndRenderAxisData(svgElement, false);
    if (!this._isSecondaryAxisSkipped()) {
      this._fetchAndRenderAxisData(svgElement, true);
    }
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

  setPrimaryAxis(axis: string) {
    this._primaryAxis = axis;
    console.log('Primary Axis set to:', this._primaryAxis);
  }

  setSecondaryAxis(axis: string) {
    this._secondaryAxis = axis;
    console.log('Secondary Axis set to:', this._secondaryAxis);
  }
  
  setTimeAxis(startTime: number, endTime: number) {
    this._startTime = startTime;
    this._endTime = endTime;
    this._updateTimeScale();
    this._widget.setXAxis(startTime, endTime);
    this._fetchAndRenderAxisData(this._svg, false);
    if (!this._isSecondaryAxisSkipped()) {
      this._fetchAndRenderAxisData(this._svg, true);
    }
  }

  _updateTimeScale() {
    this._xScale = d3
    .scaleLinear()
    .domain([this._startTime, this._endTime])
    .range([0, this._canvasWidth - this._marginLeft - this._marginRight]);
  
    this._taskRenderer.setXScale(this._xScale);
    this._drawXAxis();
    if (this._primaryYScale) {
      this._drawYAxis(this._svg, this._primaryYScale);
    }
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
    if (this._canvas) {
      const svg = d3.select(this._canvas).select<SVGSVGElement>("svg").node() as SVGElement;
      this._fetchAndRenderAxisData(svg, false);
      if (!this._isSecondaryAxisSkipped()) {
        this._fetchAndRenderAxisData(svg, true);
      }
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
    const locationLabel = document.getElementById("location-label");
    if (locationLabel) {
        locationLabel.textContent = task["where"];
    }
  }

  _removeLocation() {
    const locationLabel = document.getElementById("location-label");
    if (locationLabel) {
        locationLabel.textContent = "";
    }
  }

  setWidgetDimensions(width: number, height: number) {
    this._widget.setDimensions(width, height);
  }

  setTimeRange(startTime: number, endTime: number) {
    this._widget.setXAxis(startTime, endTime);  
    this._updateTimeScale(); 
  }

  _fetchAndRenderAxisData(svg: SVGElement, isSecondary: boolean) {
    const params = new URLSearchParams();
    params.set("info_type", "ConcurrentTask");
    params.set("where", this._componentName);
    console.log('Fetching data with componentName:', this._componentName);
    params.set("start_time", this._startTime.toString());
    params.set("end_time", this._endTime.toString());
    console.log('Fetching data time', this._startTime.toString(), this._endTime.toString());
    params.set("num_dots", this._numDots.toString());
    console.log(params.toString());
    fetch(`/api/compinfo?${params.toString()}`)
      .then((rsp) => rsp.json())
      .then((rsp) => {
        console.log('After Fetching data with componentName:', this._componentName);
        if (isSecondary) {
          this._secondaryAxisData = rsp;
        } else {
          this._primaryAxisData = rsp;
        }
        this._renderAxisData(svg, rsp, isSecondary);
      })
      .catch((error) => {
        console.error('Error fetching data:', error);
      });
  }


  _renderAxisData(svg: SVGElement, data: object, isSecondary: boolean) {
    const yScale = this._calculateYScale(data);
    if (isSecondary) {
      this._secondaryYScale = yScale;
    } else {
      this._primaryYScale = yScale;
    }

    this._drawYAxis(svg, yScale);
    this._renderDataCurve(svg, data, yScale, isSecondary);
  }

  _calculateYScale(data: Object) {
    let max = 0;

    data["data"].forEach((d: TimeValue) => {
      if (d.value > max) {
        max = d.value;
      }
    });

    const yScale = d3
      .scaleLinear()
      .domain([0, max])
      .range([this._canvasHeight - this._xAxisHeight, this._marginTop]);

    return yScale;
  }

  _drawYAxis(
    svg: SVGElement,
    yScale: d3.ScaleLinear<number, number>,
  ) {
    const canvas = d3.select(svg);

    const yAxisLeft = d3.axisLeft(yScale);
    let yAxisLeftGroup = canvas.select(".y-axis-left");
    if (yAxisLeftGroup.empty()) {
        yAxisLeftGroup = canvas.append("g").attr("class", "y-axis-left");
    }
    yAxisLeftGroup
        .attr("transform", `translate(${this._marginLeft + 35}, ${this._graphPaddingTop})`)
        .call(yAxisLeft.ticks(5, ".1e"));

    const yAxisRight = d3.axisRight(yScale);
    let yAxisRightGroup = canvas.select(".y-axis-right");
    if (yAxisRightGroup.empty()) {
        yAxisRightGroup = canvas.append("g").attr("class", "y-axis-right");
    }
    yAxisRightGroup
        .attr("transform", `translate(${this._canvasWidth - this._marginRight - 35}, ${this._graphPaddingTop})`)
        .call(yAxisRight.ticks(5, ".1e"));
  }

  _isPrimaryAxisSkipped() {
    return this._primaryAxis === "-";
  }

  _isSecondaryAxisSkipped() {
    return this._secondaryAxis === "-";
  }

  _renderDataCurve(
    svg: SVGElement,
    data: Object,
    yScale: d3.ScaleLinear<number, number>,
    isSecondary: boolean
  ) {
    const canvas = d3.select(svg);
    const className = `curve-${data["info_type"]}`;
    let reqInGroup = canvas.select(`.${className}`);
    if (reqInGroup.empty()) {
      reqInGroup = canvas.append("g").attr("class", className);
    }

    let color = "#d7191c";
    if (isSecondary) {
      color = "#2c7bb6";
    }

    const pathData = [];
    data["data"].forEach((d: TimeValue) => {
      pathData.push([d.time, d.value]);
    });

    const line = d3
      .line()
      .x((d) => this._yAxisWidth + this._xScale(d[0]))
      .y((d) => this._graphPaddingTop + yScale(d[1]))
      .curve(d3.curveCatmullRom.alpha(0.5));

    let path = reqInGroup.select(".line");
    if (path.empty()) {
      path = reqInGroup.append("path").attr("class", "line");
    }
    path
      .datum(pathData)
      .attr("d", line)
      .attr("fill", "none")
      .attr("stroke", color);

    const circles = reqInGroup.selectAll("circle").data(data["data"]);

    const circleEnter = circles
      .enter()
      .append("circle")
      .attr("cx", (d: TimeValue) => {
        const x = this._yAxisWidth + this._xScale(d.time);
        return x;
      })
      .attr("cy", this._graphContentHeight + this._graphPaddingTop)
      .attr("r", 2)
      .attr("fill", color);

    circleEnter
      .merge(
        <d3.Selection<SVGCircleElement, unknown, SVGCircleElement, unknown>>(
          circles
        )
      )
      .transition()
      .attr("cx", (d: TimeValue, i: number) => {
        const x = this._yAxisWidth + this._xScale(d.time);
        return x;
      })
      .attr("cy", (d: TimeValue) => {
        return this._graphPaddingTop + yScale(d.value);
      });

    circles.exit().remove();
  }
}

export default ComponentView;