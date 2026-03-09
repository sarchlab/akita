import { TaskColorCoder } from "./taskcolorcoder";
import { TaskPage } from "./taskpage";
import { Task } from "./task";
import * as d3 from "d3";

class Legend {
  private _canvas: HTMLElement;
  private _colorCoder: TaskColorCoder;
  private _taskPage: TaskPage;
  private _lineHeight: 28;
  private _blockHeight: 10;
  private _marginTop: 30;

  constructor(colorCoder: TaskColorCoder, taskPage: TaskPage) {
    this._canvas = null;
    this._colorCoder = colorCoder;
    this._taskPage = taskPage;

    this._lineHeight = 28;
    this._blockHeight = 10;
    this._marginTop = 30;
  }

  setCanvas(canvas: HTMLElement) {
    this._canvas = canvas;
  }

  render() {
    const colors = Object.entries(this._colorCoder.colorMap);

    const svg = d3
      .select(this._canvas)
      .select("svg")
      .attr("height", colors.length * this._lineHeight + this._marginTop);

    const colorGroups = svg.selectAll("g").data(colors, color => {
      return color[0];
    });

    const colorEnter = colorGroups
      .enter()
      .append("g")
      .on("mouseover", (_, d) => {
        this._taskPage.highlight((t: Task) => {
          const kindWhat = `${t.kind}-${t.what}`;
          return kindWhat === d[0];
        });
      })
      .on("mouseout", () => {
        this._taskPage.highlight(null);
      });

    colorEnter
      .append("rect")
      .attr("y", -(18 - this._blockHeight) / 2)
      .attr("width", 30)
      .attr("height", 10)
      .attr("stroke", "black");

    colorEnter
      .append("text")
      .attr("x", 40)
      .attr("alignment-baseline", "middle")
      .text(d => d[0]);

    const mergedGroups = colorEnter
      .merge(
        <d3.Selection<SVGGElement, [string, any], d3.BaseType, unknown>>(
          colorGroups
        )
      )
      .transition()
      .attr(
        "transform",
        (c: [string, string], i: number) =>
          `translate(5, ${i * this._lineHeight + this._marginTop})`
      )
      .selectAll("rect")
      .attr("fill", (c: [string, string]) => {
        return this._colorCoder.lookupWithText(c[0]);
      });

    colorGroups.exit().remove();
  }
}

export default Legend;