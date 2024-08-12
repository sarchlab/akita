import * as d3 from "d3";
import { ValueFn } from "d3-selection";
import TaskYIndexAssigner from "./taskyindexassigner";
import TaskRenderer from "./taskrenderer";
import XAxisDrawer from "./xaxisdrawer";
import { ScaleLinear } from "d3";
import { Task, Dim } from "./task";

interface ProgressPoint {
  x: number;
  y: number;
  time: number;
  reason: string;
}

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
  _threshold: number;
  _lastProgressPoints: Map<string, ProgressPoint>;

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

    this._threshold = 0.0000001; // 0.000004707;

    this._lastProgressPoints = new Map<string, ProgressPoint>();
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

  _renderDelays(delays: Array<{ time: number; source: string }>) {
    const svg = d3.select(this._canvas).select("svg");
    const triangles = svg
      .selectAll("polygon.delay-indicator")
      .data(delays)
      .enter()
      .append("polygon")
      .attr("class", "delay-indicator");

    triangles
      .attr("points", (d) => this._calculateTrianglePoints(d))
      .attr("fill", "red") // Use a color that indicates a delay
      .attr("stroke", "black")
      .attr("stroke-width", 1);
  }

  _calculateTrianglePoints(delay) {
    const x = this._xScale(delay.time); // Use the time to calculate the x position

    // Assuming delay.source matches a task 'where' property
    // Find the y index of the task with the matching 'where' property
    const task = this._tasks.find((task) => task.where === delay.source);

    // If no task is found, don't draw the triangle
    if (!task || !task.dim) {
      return "";
    }

    const y = task.dim.y + task.dim.height / 2; // Center the triangle in the task bar

    const size = 1; // Size of the triangle
    // Create the points for an upward-pointing triangle
    return `${x},${y - size} ${x - size},${y + size} ${x + size},${y + size}`;
  }

  _renderDelayClusters(delays: Array<{ time: number; source: string }>) {
    // Cluster delays into groups based on the threshold
    const clusters = this._clusterDelays(delays, this._threshold);
    const svg = d3.select(this._canvas).select("svg");

    // Clear any existing triangles to avoid duplicates
    svg.selectAll("polygon.delay-indicator").remove();

    const triangles = svg
      .selectAll("polygon.delay-indicator")
      .data(clusters)
      .enter()
      .append("polygon")
      .attr("class", "delay-indicator");

    triangles
      .attr("points", (d) => this._calculateTrianglePointsCluster(d))
      .attr("fill", "red") // Or use a color scale based on 'count'
      .attr("stroke", "black")
      .attr("stroke-width", 1);
  }
  _calculateTrianglePointsCluster(cluster) {
    const x = this._xScale(cluster.time); // Calculate the x position based on the cluster's time

    // Find the y index of the task with the matching 'source' property
    const task = this._tasks.find((task) => task.where === cluster.source);

    // If no task is found, don't draw the triangle
    if (!task || !task.dim) {
      return "";
    }

    // Determine the y position by using the task's dimensions
    const y = task.dim.y + task.dim.height / 2; // Center the triangle in the task bar's y position

    // Calculate the size of the triangle based on the number of delays in the cluster
    const baseSize = 3; // Base size for a cluster with a single delay
    const sizeIncrement = 1; // Increment size for each additional delay in the cluster
    const size = baseSize + sizeIncrement * (Math.sqrt(cluster.count) - 1); // Slightly increase size based on count

    // Define the points for an upward-pointing triangle, centered at the calculated x, y
    return `${x},${y - size} ${x - size},${y + size} ${x + size},${y + size}`;
  }

  _clusterDelays(
    delays: Array<{ time: number; source: string }>,
    threshold: number
  ) {
    // Sort delays by time
    const sortedDelays = delays.sort((a, b) => a.time - b.time);
    const clusters = [];
    let currentCluster = [];

    sortedDelays.forEach((delay, index) => {
      // Start a new cluster if needed
      if (
        currentCluster.length === 0 ||
        delay.time - currentCluster[currentCluster.length - 1].time <= threshold
      ) {
        currentCluster.push(delay);
      } else {
        // Current delay is outside the threshold, save and start a new cluster
        clusters.push(currentCluster);
        currentCluster = [delay];
      }

      // If it's the last delay, push the current cluster
      if (index === sortedDelays.length - 1 && currentCluster.length > 0) {
        clusters.push(currentCluster);
      }
    });

    // Convert clusters into a format suitable for rendering
    return clusters.map((cluster) => {
      // Here we take the first delay's time and source for the cluster
      return {
        time: cluster[0].time,
        source: cluster[0].source,
        count: cluster.length,
      };
    });
  }

  _renderProgress(
    progresses: Array<{
      progress_id: string;
      time: number;
      source: string;
      task_id: string;
      reason: string;
    }>
  ) {
    const svg = d3.select(this._canvas).select("svg");

    // // Clear existing progress dots and lines if necessary
    // svg.selectAll("circle.progress-indicator").remove();
    // svg.selectAll("line.progress-line").remove();

    const circles = svg
      .selectAll("circle.progress-indicator")
      .data(progresses)
      .enter()
      .append("circle")
      .attr("class", "progress-indicator")
      .attr("r", 3); // Radius of the dots

    circles
      .attr("cx", (d) => this._xScale(d.time))
      .attr("cy", (d) => this._findTaskYPosition(d.task_id))
      .attr("fill", "blue"); // Color of the progress dots

    // // Draw lines for progress
    svg
      .selectAll<
        SVGLineElement,
        {
          progress_id: string;
          time: number;
          source: string;
          task_id: string;
          reason: string;
        }
      >("line.progress-line")
      .data(progresses)
      .enter()
      .append("line")
      .attr("class", "progress-line")
      .attr("x1", (d) => {
        const task = this._tasks.find((t) => t.id === d.task_id);
        if (task && task.dim) {
          return this._xScale(task.dim.startTime);
        }
        return 0;
      })
      .attr("x2", (d) => this._xScale(d.time))
      .attr("y1", (d) => this._findTaskYPosition(d.task_id))
      .attr("y2", (d) => this._findTaskYPosition(d.task_id))
      .attr("stroke", "black")
      .attr("stroke-width", 1);
    //   .on("mouseover", function (
    //     this: SVGLineElement,
    //     event: MouseEvent,
    //     d: {
    //       progress_id: string;
    //       time: number;
    //       source: string;
    //       task_id: string;
    //       reason: string;
    //     }
    //   ) {
    //     console.log(d.reason);
    //     const tooltip = d3
    //       .select("body")
    //       .append("div")
    //       .attr("class", "reasonTooltip")
    //       .style("position", "absolute")
    //       .style("background", "lightgrey")
    //       .style("padding", "5px")
    //       .style("border", "1px solid black")
    //       .style("border-radius", "5px")
    //       .style("visibility", "hidden");

    //     tooltip
    //       .html(d.reason)
    //       .style("top", `${event.pageY - 10}px`)
    //       .style("left", `${event.pageX + 10}px`)
    //       .style("visibility", "visible");
    //   } as unknown as ValueFn<
    //     SVGLineElement,
    //     {
    //       progress_id: string;
    //       time: number;
    //       source: string;
    //       task_id: string;
    //       reason: string;
    //     },
    //     void
    //   >)
    //   .on("mouseout", function () {
    //     d3.select(".reasonTooltip").remove();
    //   }
    // );

    // Store the current progress point
    progresses.forEach((progress) => {
      this._lastProgressPoints.set(progress.task_id, {
        x: this._xScale(progress.time),
        y: this._findTaskYPosition(progress.task_id),
        time: progress.time,
        reason: progress.reason,
      });
    });
  }

  _findTaskYPosition(taskId) {
    const task = this._tasks.find((t) => t.id === taskId);
    if (!task || !task.dim) {
      return -1; // Return an off-screen value if task is not found
    }
    return task.dim.y + task.dim.height / 2; // Center the dot vertically within the task bar
  }

  private rearrangeAsTree(tasks: Array<Task>): Task {
    const virtualRoot = new Task();
    virtualRoot.level = 0;

    const forest = new Array<Task>();
    const idToTaskMap = {};

    tasks.forEach((task) => {
      task.subTasks = [];
      idToTaskMap[task.id] = task;
    });

    tasks.forEach((task) => {
      const parentTask = idToTaskMap[task.parent_id];
      if (!parentTask) {
        forest.push(task);
      } else {
        parentTask.subTasks.push(task);
      }
    });

    forest.forEach((t) => {
      this.assignTaskLevel(t, 1);
    });

    virtualRoot.subTasks = forest;

    return virtualRoot;
  }

  private assignTaskLevel(task: Task, level: number) {
    task.level = level;
    task.subTasks.forEach((t) => {
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
    tasksOfLevel.forEach((t) => {
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

    tasksOfLevel.forEach((t) => {
      const paddedDim = Object.assign({}, t.dim);
      // paddedDim.height -= 6;
      // paddedDim.y += 3;

      const subTasks = t.subTasks;
      subTasks.forEach((subT) => {
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

    tree.subTasks.forEach((t) => {
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
    return tasks.filter((t) => {
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
