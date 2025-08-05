import * as d3 from "d3";
class Legend {
    constructor(colorCoder, taskPage) {
        this._canvas = null;
        this._colorCoder = colorCoder;
        this._taskPage = taskPage;
        this._lineHeight = 28;
        this._blockHeight = 10;
        this._marginTop = 30;
    }
    setCanvas(canvas) {
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
            this._taskPage.highlight((t) => {
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
            .merge((colorGroups))
            .transition()
            .attr("transform", (c, i) => `translate(5, ${i * this._lineHeight + this._marginTop})`)
            .selectAll("rect")
            .attr("fill", (c) => {
            return this._colorCoder.lookupWithText(c[0]);
        });
        colorGroups.exit().remove();
    }
}
export default Legend;
