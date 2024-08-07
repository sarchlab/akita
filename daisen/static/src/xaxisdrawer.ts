import * as d3 from "d3";

class XAxisDrawer {
  _axisHeight: number;
  _marginLeft: number;
  _canvas: HTMLElement;
  _canvasWidth: number;
  _canvasHeight: number;
  _xScale: d3.ScaleLinear<number, number>;
  _startTime: number;
   _endTime: number;

  constructor() {
    this._axisHeight = 20;
    this._marginLeft = 0;
    this._canvas = null;
    this._xScale = null;
    this._startTime = 0;
    this._endTime = 0;
  }

  setCanvas(canvas: HTMLElement) {
    this._canvas = canvas;
    return this;
  }

  setMarginLeft(px: number) {
    this._marginLeft = px;
    return this;
  }

  setCanvasWidth(px: number) {
    this._canvasWidth = px;
    return this;
  }

  setCanvasHeight(px: number) {
    this._canvasHeight = px;
    return this;
  }

  setScale(scale: d3.ScaleLinear<number, number>) {
    this._xScale = scale;
    return this;
  }

  setTimeRange(startTime: number, endTime: number) {
    this._startTime = startTime;
    this._endTime = endTime;
    this._updateScale();
    return this;
  }

  _updateScale() {
    this._xScale = d3.scaleLinear()
      .domain([this._startTime, this._endTime])
      .range([this._marginLeft, this._canvasWidth - this._marginLeft]);
  }

  renderTop(yOffset = 0) {
    const xAxis = d3.axisTop(this._xScale);
    const svg = d3.select(this._canvas).select("svg");
    let xAxisGroup = svg.select(".x-axis-top");
    let rect = null;
    if (xAxisGroup.empty()) {
      xAxisGroup = svg.append("g").attr("class", "x-axis-top");
      rect = xAxisGroup.append("rect");
    } else {
      rect = xAxisGroup.select("rect");
    }
    const xAxisTop = this._axisHeight + yOffset;
    xAxisGroup
      .attr("transform", `translate(${this._marginLeft}, ${xAxisTop})`)
      .call(xAxis.ticks(12, "s"));
    rect
      .attr("fill", "#ffffff")
      .attr("height", this._axisHeight)
      .attr("width", this._canvasWidth)
      .attr("x", 0)
      .attr("y", -xAxisTop + yOffset);

    return this;
  }

  renderBottom(yOffset = 0) {
    const xAxis = d3.axisBottom(this._xScale);
    const svg = d3.select(this._canvas).select("svg");
    let xAxisGroup = svg.select(".x-axis-bottom");
    let rect = null;
    if (xAxisGroup.empty()) {
      xAxisGroup = svg.append("g").attr("class", "x-axis-bottom");
      rect = xAxisGroup.append("rect");
    } else {
      rect = xAxisGroup.select("rect");
    }

    const xAxisTop = this._canvasHeight - this._axisHeight + yOffset;
    xAxisGroup
      .attr("transform", `translate(${this._marginLeft}, ${xAxisTop})`)
      .call(xAxis.ticks(12, "s"));
    rect
      .attr("fill", "#ffffff")
      .attr("height", this._axisHeight)
      .attr("width", this._canvasWidth)
      .attr("x", 0)
      .attr("y", 0);

    return this;
  }

  renderCustom(yOffset: number) {
    const xAxis = d3.axisBottom(this._xScale);
    const svg = d3.select(this._canvas).select("svg");
    let xAxisGroup = svg.select(".x-axis-custom");
    let rect = null;
    if (xAxisGroup.empty()) {
      xAxisGroup = svg.append("g").attr("class", "x-axis-custom");
      rect = xAxisGroup.append("rect");
    } else {
      rect = xAxisGroup.select("rect");
    }
  
    const safeYOffset = Math.min(yOffset, this._canvasHeight - this._axisHeight);

    xAxisGroup
      .attr("transform", `translate(${this._marginLeft}, ${yOffset})`)
      .call(xAxis.ticks(12, "s"));

    const tickValues = this._xScale.ticks(12);
    const dashedLines = xAxisGroup.selectAll(".tick-line")
      .data(tickValues);

    dashedLines.enter()
    .append("line")
    .attr("class", "tick-line")
    .merge(dashedLines as any)
    .attr("x1", d => this._xScale(d))
    .attr("x2", d => this._xScale(d))
    .attr("y1", -safeYOffset)
    .attr("y2", this._canvasHeight - safeYOffset)
    .attr("stroke", "#000")
    .attr("stroke-dasharray", "3,3")
    .attr("opacity", 0.5);

    dashedLines.exit().remove();
    xAxisGroup.selectAll("path").attr("stroke", "black");
    xAxisGroup.selectAll("line").attr("stroke", "black");
    xAxisGroup.selectAll("text")
      .attr("fill", "black")
      .attr("font-size", "12px");
    rect
      .attr("fill", "none") 
      .attr("height", this._axisHeight)
      .attr("width", this._canvasWidth)
      .attr("x", 0)
      .attr("y", -this._axisHeight); 
  }
  
}

export default XAxisDrawer;
