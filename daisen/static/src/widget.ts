import * as d3 from "d3";
import Dashboard from "./dashboard";
import TaskPage from "./taskpage";
import { ZoomHandler, MouseEventHandler } from "./mouseeventhandler";

export class TimeValue {
  time: number;
  value: number;

  constructor(time: number, value: number) {
    this.time = time;
    this.value = value;
  }
}

type DataObject = {
  info_type: string;
  data: TimeValue[];
};

export class Widget implements ZoomHandler {
  _dashboard: Dashboard;
  _componentName: string;
  _div: HTMLDivElement;
  _canvas: HTMLDivElement;
  _svg: SVGElement;
  _mouseEventHandler: MouseEventHandler;
  _thumbnail: HTMLDivElement;
  _taskPage: TaskPage;

  _numDots: number;
  _startTime: number;
  _endTime: number;
  _primaryAxis: string;
  _secondaryAxis: string;
  _primaryAxisData: object;
  _secondaryAxisData: object;
  _scrollingTimer: number;

  _widgetHeight: number;
  _widgetWidth: number;
  _yAxisWidth: number;
  _xAxisHeight: number;
  _graphWidth: number;
  _graphHeight: number;
  _graphContentWidth: number;
  _graphContentHeight: number;
  _titleHeight: number;
  _graphPaddingTop: number;

  _xScale: d3.ScaleLinear<number, number>;
  _yScale: d3.ScaleLinear<number, number>;
  _primaryYScale: d3.ScaleLinear<number, number>;
  _secondaryYScale: d3.ScaleLinear<number, number>;

  constructor(
    componentName: string,
    canvas: HTMLDivElement,
    dashboard: Dashboard
  ) {
    this._dashboard = dashboard;
    this._componentName = componentName;
    console.log('Widget created for component:', this._componentName);
    this._canvas = canvas;

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

    this._startTime = 0;
    this._endTime = 0;
    this._primaryAxis = "ReqInCount";
    this._secondaryAxis = "AvgLatency";
    this._xScale = null;
  }

  setSVG(svg: SVGElement) {
    this._svg = svg;
  }
  
  public async initialize(): Promise<void> {
    this._svg = await this.loadSvgElement();
  }

  private async loadSvgElement(): Promise<SVGElement> {
    return new Promise(resolve => {
        setTimeout(() => {
            const svgElement = document.createElementNS("http://www.w3.org/2000/svg", "svg");
            svgElement.setAttribute("width", "100");
            svgElement.setAttribute("height", "50");
            resolve(svgElement);
        }, 1000);
    });
  }
  setDimensions(width: number, height: number) {
    this._widgetWidth = width;
    this._widgetHeight = height;
    this._graphWidth = this._widgetWidth;
    this._graphContentWidth = this._widgetWidth - 2 * this._yAxisWidth;
    this._graphHeight = this._widgetHeight - this._titleHeight;
    this._graphContentHeight = this._graphHeight - 
    this._xAxisHeight - this._graphPaddingTop;
  }
  
  resize(width: number, height: number) {
    this.setDimensions(width, height);
    this._renderXAxis(this._svg);
    if (!this._isPrimaryAxisSkipped()) {
      this._renderDataCurve(
        this._svg,
        this._primaryAxisData,
        this._primaryYScale,
        false
      );
      this._drawYAxis(this._svg, this._primaryYScale, false);
    }
  
    if (!this._isSecondaryAxisSkipped()) {
      this._renderDataCurve(
        this._svg,
        this._secondaryAxisData,
        this._secondaryYScale,
        true
      );
      this._drawYAxis(this._svg, this._secondaryYScale, true);
    }
  }

  domElement(): SVGElement {
    return this._svg;
  }

  getAxisStatus(): [number, number, number, number] {
    return [this._startTime, this._endTime, 0, this._widgetWidth];
  }

  setXAxis(startTime: number, endTime: number) {
    this._startTime = startTime;
    this._endTime = endTime;

    this._xScale = d3
      .scaleLinear()
      .domain([this._startTime, this._endTime])
      .range([0, this._graphContentWidth]);
  }

  setYScale(yScale: d3.ScaleLinear<number, number>) {
    this._yScale = yScale;
  }
  
  temporaryTimeShift(startTime: number, endTime: number) {
    this.setXAxis(startTime, endTime);
    this._renderXAxis(this._svg);
    if (!this._isPrimaryAxisSkipped()) {
      this._renderDataCurve(
        this._svg,
        this._primaryAxisData,
        this._primaryYScale,
        false
      );
    }

    if (!this._isSecondaryAxisSkipped()) {
      this._renderDataCurve(
        this._svg,
        this._secondaryAxisData,
        this._secondaryYScale,
        true
      );
    }
  }

  permanentTimeShift(startTime: number, endTime: number) {
    this.temporaryTimeShift(startTime, endTime);
    this._dashboard.setTimeRange(startTime, endTime);
  }

  setFirstAxis(firstAxisName: string) {
    this._primaryAxis = firstAxisName;
  }

  setSecondAxis(secondAxisName: string) {
    this._secondaryAxis = secondAxisName;
  }

  _setWidgetWidth(width: number) {
    this._div.style.width = width.toString() + "px";
    this._widgetWidth = width - 8;
    this._graphWidth = this._widgetWidth;
    this._graphContentWidth = this._graphWidth - 2 * this._yAxisWidth;

    this._xScale = d3
      .scaleLinear()
      .domain([this._startTime, this._endTime])
      .range([0, this._graphContentWidth]);
  }

  _setWidgetHeight(height: number) {
    this._div.style.height = height.toString() + "px";
    this._widgetHeight = height - 14;
    this._graphHeight = this._widgetHeight - this._titleHeight;
    this._graphContentHeight =
      this._graphHeight - this._xAxisHeight - this._graphPaddingTop;

    if (this._primaryAxisData) {
      this._primaryYScale = this._calculateYScale(this._primaryAxisData);
    }

    if (this._secondaryAxisData) {
      this._secondaryYScale = this._calculateYScale(this._secondaryAxisData);
    }
  }

  render(reset: boolean) {
    let svg = this._svg;

    if (reset) {
      this._div.innerHTML = "";
      this._createTitle(this._div);
      svg = this._createSVG(this._div);

      this._mouseEventHandler = new MouseEventHandler(this);
      this._mouseEventHandler.register(this);
    }
    d3.select(this._svg)
    .attr("width", this._widgetWidth)
    .attr("height", this._widgetHeight);
    this._renderXAxis(svg);
    this._fetchAndRenderAxisData(svg, true);
    this._fetchAndRenderAxisData(svg, false);
  }

  setXScale(xScale: d3.ScaleLinear<number, number>) {
    this._xScale = xScale;
  }
  
  createWidget(width: number, height: number) {
    const div = document.createElement("div");

    div.classList.add("widget");
    this._canvas.appendChild(div);
    this._div = div;

    this._setWidgetWidth(width);
    this._setWidgetHeight(height);

    return div;
  }

  _createTitle(div: HTMLDivElement) {
    const titleBar = document.createElement("div");
    titleBar.classList.add("title-bar");
    div.appendChild(titleBar);

    const title = document.createElement("h6");
    title.innerHTML = this._componentName;
    titleBar.appendChild(title);

    title.onclick = () => {
      window.location.href = `/component?name=${this._componentName}&starttime=${this._startTime}&endtime=${this._endTime}`;
    };

    this._createSaveButton(titleBar);
  }
  
  _createSaveButton(titleBar: HTMLDivElement) {
    const btn = document.createElement("div");
    btn.classList.add("btn");
    btn.innerHTML = `<i class="far fa-save"></i>`;

    titleBar.appendChild(btn);

    btn.onclick = () => {
      this._svg.setAttribute("xmlns", "http://www.w3.org/2000/svg");
      const svgData = this._svg.outerHTML;
      const preface = '<?xml version="1.0" standalone="no"?>\r\n';
      const svgBlob = new Blob([preface, svgData], {
        type: "image/svg+xml;charset=utf-8",
      });
      const svgUrl = URL.createObjectURL(svgBlob);
      const downloadLink = document.createElement("a");
      downloadLink.href = svgUrl;
      // downloadLink.download = name;
      document.body.appendChild(downloadLink);
      downloadLink.click();
      document.body.removeChild(downloadLink);
    };
  }

  _createSVG(div: HTMLDivElement) {
    const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg") as SVGSVGElement;;
    svg.setAttribute("width", "100%");
    svg.setAttribute("height", this._widgetHeight.toString());
    div.appendChild(svg);
    // this._addMouseListener(svg)

    this._svg = svg;
    return svg;
  }

  _triggerZoom() {
    this._dashboard.setTimeRange(this._startTime, this._endTime);
  }

  _renderXAxis(svg: SVGElement) {
    d3.select(svg).selectAll(".x-axis-bottom").remove()
    this._drawXAxis(svg, this._xScale);
  }

  _drawXAxis(svg: SVGElement, xScale: d3.ScaleLinear<number, number>) {
    const canvas = d3.select(svg);
    const xAxis = d3.axisBottom(xScale);
    let xAxisGroup = canvas.select(".x-axis-bottom");
    if (xAxisGroup.empty()) {
      xAxisGroup = canvas.append("g").attr("class", "x-axis-bottom");
    }
    const xAxisTop = this._widgetHeight - this._titleHeight - this._xAxisHeight;
    xAxisGroup.attr("transform", `translate(${this._yAxisWidth}, ${xAxisTop})`);

    xAxisGroup.call(xAxis.ticks(5, "s"));
  }

  _fetchAndRenderAxisData(svg: SVGElement, isSecondary: boolean) {
    const params = new URLSearchParams();
    if (isSecondary) {
      params.set("info_type", this._secondaryAxis);
    } else {
      params.set("info_type", this._primaryAxis);
    }
    params.set("where", this._componentName);
    params.set("start_time", this._startTime.toString());
    params.set("end_time", this._endTime.toString());
    params.set("num_dots", this._numDots.toString());
    fetch(`/api/compinfo?${params.toString()}`)
      .then((rsp) => rsp.json())
      .then((rsp) => {
        if (isSecondary) {
          this._secondaryAxisData = rsp;
        } else {
          this._primaryAxisData = rsp;
        }
        this._renderAxisData(svg, rsp, isSecondary);
      });
  }

  _renderAxisData(svg: SVGElement, data: object, isSecondary: boolean) {
    const yScale = this._calculateYScale(data);
    if (isSecondary) {
      this._secondaryYScale = yScale;
      d3.select(svg).selectAll(".y-axis-right").remove();
    } else {
      this._primaryYScale = yScale;
      d3.select(svg).selectAll(".y-axis-left").remove(); 
    }

    this._drawYAxis(svg, yScale, isSecondary);
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
      .range([this._graphContentHeight, 0]);

    return yScale;
  }

  _drawYAxis(
    svg: SVGElement,
    yScale: d3.ScaleLinear<number, number>,
    isSecondary: boolean
  ) {
    const canvas = d3.select(svg);

    let yAxis = d3.axisLeft(yScale);
    let xOffset = this._yAxisWidth;
    let axisClass = "y-axis-left";
    if (isSecondary) {
      yAxis = d3.axisRight(yScale);
      xOffset += this._graphContentWidth;
      axisClass = "y-axis-right";
    }

    let yAxisGroup = canvas.select("." + axisClass);
    if (yAxisGroup.empty()) {
      yAxisGroup = canvas.append("g").attr("class", axisClass);
    }
    yAxisGroup.attr(
      "transform",
      `translate(${xOffset}, ${this._graphPaddingTop})`
    );
    yAxisGroup.call(yAxis.ticks(5, ".1e"));
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
    canvas.selectAll(`.${className}`).remove();
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

  clear() {
    d3.select(this._svg).selectAll("*").remove();
  }

}

export default Widget;