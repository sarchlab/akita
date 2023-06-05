import * as d3 from "d3";

class XAxisDrawer {
  _axisHeight: number;
  _marginLeft: number;
  _canvas: HTMLElement;
  _canvasWidth: number;
  _canvasHeight: number;
  _xScale: d3.ScaleLinear<number, number>;

  constructor() {
    this._axisHeight = 20;
    this._marginLeft = 0;
    this._canvas = null;
    this._xScale = null;
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
}

export default XAxisDrawer;
