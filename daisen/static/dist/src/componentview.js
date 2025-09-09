import * as d3 from "d3";
import { Task, Dim } from "./task";
class ComponentView {
    constructor(yIndexAssigner, taskRenderer, xAxisDrawer, widget) {
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
        this._graphPaddingTop = 0;
        this._xAxisHeight = 30;
        this._graphContentHeight =
            this._graphHeight - this._xAxisHeight - this._graphPaddingTop;
        this._componentName = this._widget._componentName;
        this._startTime = 0;
        this._endTime = 0;
        this._primaryAxis = "ConcurrentTask";
        this._xScale = null;
    }
    setComponentName(componentName) {
        this._componentName = componentName;
        if (this._canvas) {
            const svg = d3.select(this._canvas).select("svg").node();
            this._fetchAndRenderAxisData(svg);
        }
        console.log('Component Name set to:', this._componentName);
    }
    setCanvas(canvas, tooltip) {
        this._canvas = canvas;
        this._canvasWidth = this._canvas.offsetWidth;
        this._canvasHeight = this._canvas.offsetHeight;
        this._taskRenderer.setCanvas(canvas, tooltip);
        this._xAxisDrawer.setCanvas(canvas);
        const svg = d3.select(this._canvas)
            .select("svg")
            .attr("width", this._canvasWidth)
            .attr("height", this._canvasHeight);
        const svgElement = svg.node();
        this._updateTimeScale();
        this._renderData();
        this._fetchAndRenderAxisData(svgElement);
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
    setPrimaryAxis(axis) {
        this._primaryAxis = axis;
        console.log('Primary Axis set to:', this._primaryAxis);
    }
    setTimeAxis(startTime, endTime) {
        this._startTime = startTime;
        this._endTime = endTime;
        this._updateTimeScale();
        this._widget.setXAxis(startTime, endTime);
        this._fetchAndRenderAxisData(this._svg);
    }
    _updateTimeScale() {
        this._xScale = d3
            .scaleLinear()
            .domain([this._startTime, this._endTime])
            .range([this._marginLeft, this._canvasWidth - this._marginLeft]);
        this._taskRenderer.setXScale(this._xScale);
        this._drawXAxis();
    }
    _drawXAxis() {
        this._xAxisDrawer
            .setCanvasHeight(this._canvasHeight)
            .setCanvasWidth(this._canvasWidth)
            .setScale(this._xScale)
            .renderCustom(this._canvasHeight - 20);
    }
    updateXAxis() {
        this._taskRenderer.updateXAxis();
    }
    highlight(task) {
        this._taskRenderer.hightlight(task);
    }
    render(tasks) {
        this._tasks = tasks;
        this._renderData();
        if (tasks.length > 0) {
            this._showLocation(tasks[0]);
            this.setComponentName(tasks[0].location);
        }
        else {
            this._removeLocation();
        }
        if (this._canvas) {
            const svg = d3.select(this._canvas).select("svg").node();
            this._fetchAndRenderAxisData(svg);
        }
    }
    _renderData() {
        if (!this._tasks) {
            return;
        }
        let tasks = this._tasks;
        const tree = this.rearrangeAsTree(tasks);
        const initDim = new Dim();
        initDim.x = 0;
        initDim.y = 0;
        initDim.width = this._canvasWidth;
        initDim.height = 2 * (this._canvasHeight - this._marginTop - this._marginBottom - 50) / 3;
        initDim.startTime = this._startTime;
        initDim.endTime = this._endTime;
        this.assignDimension(tree, initDim);
        // this._normalizeTaskHeight(tasks);
        tasks.sort((a, b) => a.level - b.level);
        tasks = this._filterTasks(tasks);
        this._taskRenderer
            .renderWithX((t) => t.dim.x)
            .renderWithY((t) => t.dim.y)
            .renderWithHeight((t) => t.dim.height)
            .renderWithWidth((t) => t.dim.width)
            .render(tasks);
    }
    rearrangeAsTree(tasks) {
        const virtualRoot = new Task();
        virtualRoot.level = 0;
        const forest = new Array();
        const idToTaskMap = {};
        tasks.forEach(task => {
            task.subTasks = [];
            idToTaskMap[task.id] = task;
        });
        tasks.forEach(task => {
            const parentTask = idToTaskMap[task.parent_id];
            if (!parentTask) {
                forest.push(task);
            }
            else {
                parentTask.subTasks.push(task);
            }
        });
        forest.forEach(t => {
            this.assignTaskLevel(t, 1);
        });
        virtualRoot.subTasks = forest;
        return virtualRoot;
    }
    assignTaskLevel(task, level) {
        task.level = level;
        task.subTasks.forEach(t => {
            this.assignTaskLevel(t, level + 1);
        });
    }
    assignDimension(tree, containerDim) {
        let taskHeight = containerDim.height;
        let depth = 0;
        tree.dim = containerDim;
        while (taskHeight > 0) {
            taskHeight = this.assignDimensionLevel(tree, taskHeight, depth);
            depth++;
        }
    }
    assignDimensionLevel(tree, parentLevelHeight, depth) {
        if (parentLevelHeight < 2) {
            return 0;
        }
        const tasksOfLevel = new Array();
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
        const paddedTaskHeight = this.padTaskHeight(taskHeight) / 2;
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
    getTasksAtLevel(tree, depth, returnTasks) {
        if (depth === 0) {
            returnTasks.push(tree);
            return;
        }
        tree.subTasks.forEach(t => {
            this.getTasksAtLevel(t, depth - 1, returnTasks);
        });
    }
    padTaskHeight(height) {
        if (height > 10) {
            return height * 0.8;
        }
        else {
            return height * 0.6;
        }
    }
    _filterTasks(tasks) {
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
    _showLocation(task) {
        const locationLabel = document.getElementById("location-label");
        if (locationLabel) {
            locationLabel.textContent = task["location"];
        }
    }
    _removeLocation() {
        const locationLabel = document.getElementById("location-label");
        if (locationLabel) {
            locationLabel.textContent = "";
        }
    }
    setWidgetDimensions(width, height) {
        this._widget.setDimensions(width, height);
    }
    setTimeRange(startTime, endTime) {
        this._widget.setXAxis(startTime, endTime);
        this._updateTimeScale();
    }
    _fetchAndRenderAxisData(svg) {
        if (!this._componentName || this._startTime >= this._endTime) {
            console.error('Invalid parameters for fetching data');
            return;
        }
        // Fetch both regular concurrent tasks and milestone-based stacked data
        const params1 = new URLSearchParams();
        params1.set("info_type", "ConcurrentTask");
        params1.set("where", this._componentName);
        params1.set("start_time", this._startTime.toString());
        params1.set("end_time", this._endTime.toString());
        params1.set("num_dots", this._numDots.toString());
        const params2 = new URLSearchParams();
        params2.set("info_type", "ConcurrentTaskMilestones");
        params2.set("where", this._componentName);
        params2.set("start_time", this._startTime.toString());
        params2.set("end_time", this._endTime.toString());
        params2.set("num_dots", this._numDots.toString());
        console.log('Fetching data with componentName:', this._componentName);
        console.log('Fetching data time', this._startTime.toString(), this._endTime.toString());
        // First fetch the regular data (this must work)
        fetch(`/api/compinfo?${params1.toString()}`)
            .then(rsp => rsp.json())
            .then((regularData) => {
            console.log('After Fetching regular data with componentName:', this._componentName);
            this._primaryAxisData = regularData;
            this._renderAxisData(svg, regularData);
            // Then try to fetch stacked data as an enhancement
            return fetch(`/api/compinfo?${params2.toString()}`);
        })
            .then(rsp => rsp.json())
            .then((stackedData) => {
            console.log('Successfully fetched stacked data');
            this._renderStackedMilestones(svg, stackedData);
        })
            .catch((error) => {
            console.log('Error fetching stacked data (this is optional):', error);
            // The regular line chart should still be displayed even if stacked data fails
        });
    }
    _renderAxisData(svg, data) {
        const yScale = this._calculateYScale(data);
        this._primaryYScale = yScale;
        this._drawYAxis(svg, yScale);
        this._renderDataCurve(svg, data, yScale);
    }
    _calculateYScale(data) {
        let max = 0;
        data["data"].forEach((d) => {
            if (d.value > max) {
                max = d.value;
            }
        });
        const yScale = d3
            .scaleLinear()
            .domain([0, max])
            .range([this._canvasHeight - this._xAxisHeight + 5, this._marginTop + 2 * (this._canvasHeight - this._xAxisHeight - this._marginTop) / 3 - 15]);
        return yScale;
    }
    _drawYAxis(svg, yScale) {
        const canvas = d3.select(svg);
        const yAxisLeft = d3.axisLeft(yScale);
        let yAxisLeftGroup = canvas.select(".y-axis-left");
        if (yAxisLeftGroup.empty()) {
            yAxisLeftGroup = canvas.append("g").attr("class", "y-axis-left");
        }
        const tickValues = yScale.ticks(5);
        const gridLines = yAxisLeftGroup.selectAll(".grid-line")
            .data(tickValues);
        gridLines.enter()
            .append("line")
            .attr("class", "grid-line")
            .merge(gridLines)
            .attr("x1", 0)
            .attr("x2", this._canvasWidth - this._marginLeft - 35)
            .attr("y1", d => yScale(d))
            .attr("y2", d => yScale(d))
            .attr("stroke", "#ccc")
            .attr("stroke-dasharray", "3,3")
            .attr("opacity", 0.5);
        gridLines.exit().remove();
        yAxisLeftGroup
            .attr("transform", `translate(${this._marginLeft + 35}, ${this._graphPaddingTop})`)
            .call(yAxisLeft.ticks(5, ".1e"));
        yAxisLeftGroup.selectAll(".domain")
            .attr("opacity", 0);
        yAxisLeftGroup.selectAll(".tick line")
            .attr("opacity", 0);
    }
    _isPrimaryAxisSkipped() {
        return this._primaryAxis === "-";
    }
    _renderDataCurve(svg, data, yScale) {
        const canvas = d3.select(svg);
        const className = `curve-${data["info_type"]}`;
        let reqInGroup = canvas.select(`.${className}`);
        if (reqInGroup.empty()) {
            reqInGroup = canvas.append("g").attr("class", className);
        }
        let color = "#2c7bb6";
        const pathData = [];
        data["data"].forEach((d) => {
            pathData.push([d.time, d.value]);
        });
        const line = d3
            .line()
            .x((d) => this._xScale(d[0]))
            .y((d) => yScale(d[1]))
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
    }
    _renderStackedMilestones(svg, stackedData) {
        if (!stackedData || !stackedData.data || !stackedData.kinds) {
            console.log('No stacked milestone data available');
            return;
        }
        const canvas = d3.select(svg);
        // Remove existing stacked bars
        canvas.selectAll(".stacked-milestones").remove();
        const stackedGroup = canvas.append("g").attr("class", "stacked-milestones");
        // Color scheme for different milestone kinds
        const colorScale = d3.scaleOrdinal(d3.schemeCategory10);
        // Calculate bar width - make bars thinner so they don't interfere with the line
        const barWidth = Math.min(8, (this._canvasWidth - this._marginLeft * 2) / stackedData.data.length * 0.3);
        // Calculate the maximum total value for scaling
        let maxTotal = 0;
        stackedData.data.forEach((d) => {
            let total = 0;
            stackedData.kinds.forEach((kind) => {
                total += d.values[kind] || 0;
            });
            if (total > maxTotal) {
                maxTotal = total;
            }
        });
        // Create separate scale for stacked bars - use secondary y-axis range (right side)
        const stackedYScale = d3
            .scaleLinear()
            .domain([0, maxTotal])
            .range([this._canvasHeight - this._xAxisHeight + 5, this._marginTop + 2 * (this._canvasHeight - this._xAxisHeight - this._marginTop) / 3 - 15]);
        // Draw stacked bars for each time point
        stackedData.data.forEach((timePoint, index) => {
            const x = this._xScale(timePoint.time) - barWidth / 2;
            let currentY = this._canvasHeight - this._xAxisHeight + 5; // Start from bottom
            stackedData.kinds.forEach((kind, kindIndex) => {
                const value = timePoint.values[kind] || 0;
                if (value > 0) {
                    const barHeight = Math.abs(stackedYScale(value) - stackedYScale(0));
                    stackedGroup
                        .append("rect")
                        .attr("x", x)
                        .attr("y", currentY - barHeight)
                        .attr("width", barWidth)
                        .attr("height", barHeight)
                        .attr("fill", colorScale(kind))
                        .attr("opacity", 0.6) // More transparent so line shows through
                        .attr("stroke", "#fff")
                        .attr("stroke-width", 0.5)
                        .on("mouseover", (event) => {
                        // Show tooltip with milestone kind and count
                        const tooltip = d3.select("body")
                            .append("div")
                            .attr("class", "milestone-stack-tooltip")
                            .style("position", "absolute")
                            .style("background", "rgba(0,0,0,0.8)")
                            .style("color", "white")
                            .style("padding", "5px 10px")
                            .style("border-radius", "3px")
                            .style("font-size", "12px")
                            .style("pointer-events", "none")
                            .style("z-index", "1000")
                            .html(`<strong>${kind}</strong><br/>Count: ${value}<br/>Time: ${timePoint.time.toFixed(3)}s`);
                        tooltip
                            .style("left", (event.pageX + 10) + "px")
                            .style("top", (event.pageY - 10) + "px");
                    })
                        .on("mouseout", () => {
                        d3.selectAll(".milestone-stack-tooltip").remove();
                    });
                    currentY -= barHeight;
                }
            });
        });
        // Add second y-axis for stacked bars on the right
        this._drawSecondaryYAxis(svg, stackedYScale, maxTotal);
        // Add legend for milestone kinds
        this._renderStackedLegend(svg, stackedData.kinds, colorScale);
    }
    _drawSecondaryYAxis(svg, yScale, maxValue) {
        const canvas = d3.select(svg);
        // Remove existing secondary y-axis
        canvas.selectAll(".y-axis-right").remove();
        const yAxisRight = d3.axisRight(yScale);
        let yAxisRightGroup = canvas.append("g").attr("class", "y-axis-right");
        yAxisRightGroup
            .attr("transform", `translate(${this._canvasWidth - this._marginRight - 35}, ${this._graphPaddingTop})`)
            .call(yAxisRight.ticks(5));
        // Style the secondary axis
        yAxisRightGroup.selectAll(".domain")
            .attr("stroke", "#666")
            .attr("opacity", 0.7);
        yAxisRightGroup.selectAll(".tick line")
            .attr("stroke", "#666")
            .attr("opacity", 0.7);
        yAxisRightGroup.selectAll(".tick text")
            .attr("fill", "#666")
            .attr("font-size", "10px");
        // Add axis label
        yAxisRightGroup
            .append("text")
            .attr("transform", "rotate(90)")
            .attr("y", -50)
            .attr("x", (this._canvasHeight - this._xAxisHeight) / 2)
            .attr("text-anchor", "middle")
            .attr("fill", "#666")
            .attr("font-size", "12px")
            .text("Milestone Count");
    }
    _renderStackedLegend(svg, kinds, colorScale) {
        const canvas = d3.select(svg);
        // Remove existing legend
        canvas.selectAll(".stacked-legend").remove();
        const legendGroup = canvas.append("g").attr("class", "stacked-legend");
        const legendX = this._canvasWidth - 200;
        const legendY = this._marginTop + 50;
        const itemHeight = 18;
        // Add legend background
        legendGroup
            .append("rect")
            .attr("x", legendX - 10)
            .attr("y", legendY - 15)
            .attr("width", 140)
            .attr("height", kinds.length * itemHeight + 20)
            .attr("fill", "white")
            .attr("stroke", "#ccc")
            .attr("stroke-width", 1)
            .attr("opacity", 0.9);
        // Add legend title
        legendGroup
            .append("text")
            .attr("x", legendX)
            .attr("y", legendY)
            .attr("font-size", "12px")
            .attr("font-weight", "bold")
            .text("Milestone Kinds");
        // Add legend items
        kinds.forEach((kind, index) => {
            const itemY = legendY + 15 + index * itemHeight;
            // Color square
            legendGroup
                .append("rect")
                .attr("x", legendX)
                .attr("y", itemY - 8)
                .attr("width", 12)
                .attr("height", 12)
                .attr("fill", colorScale(kind))
                .attr("opacity", 0.6);
            // Text label
            legendGroup
                .append("text")
                .attr("x", legendX + 18)
                .attr("y", itemY)
                .attr("font-size", "10px")
                .attr("alignment-baseline", "middle")
                .text(kind);
        });
    }
}
export default ComponentView;
