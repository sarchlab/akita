import * as d3 from "d3";

class TaskChart {
    constructor() {
        this.data = null;
        this.canvas = null;
        this.canvasWidth = 0;
        this.canvasHeight = 0;
    }

    updateData(data) {
        this.data = data;
        this.render();
    }

    render() {
        this.calculateDimension();

        this.svg = d3.select(this.canvas)
            .select('svg')
            .attr('width', this.canvasWidth)
            .attr('height', this.canvasHeight);

        this.calculateScale();

        this.renderAxis();
    }

    calculateDimension() {
        this.canvasWidth = this.canvas.offsetWidth;
        this.canvasHeight = this.canvas.offsetHeight;
        this.marginLeft = 50;
        this.marginRight = 10;
        this.marginTop = 50;
        this.marginBottom = -50;
        this.mainHeight = this.canvasHeight -
            this.marginBottom -
            this.marginTop;
        this.mainWidth = this.canvasWidth -
            this.marginLeft -
            this.marginRight;
    }

    calculateScale() {
        let earliestTime = this.findEarliestTime();
        let latestTime = this.findLatestTime();

        this.xScale = d3.scaleLinear()
            .domain([earliestTime, latestTime])
            .range([this.marginLeft, this.marginLeft + this.mainWidth]);
    }

    findEarliestTime() {
        let ealiestTime = Number.MAX_VALUE;
        ealiestTime = this.data.reduce((earliestTime, d) => {
            if (d.start_time < earliestTime) {
                earliestTime = d.start_time;
            }
            return earliestTime;
        }, ealiestTime);
        return ealiestTime;
    }

    findLatestTime() {
        let latestTime = Number.MIN_VALUE;
        latestTime = this.data.reduce((latestTime, d) => {
            if (d.end_time > latestTime) {
                latestTime = end_time;
            }
            return latestTime;
        }, latestTime);
        return latestTime;
    }

    renderAxis() {
        const xAxis = d3.axisBottom(this.xScale);
        let xAxisGroup = this.svg.select('x-axis');
        if (xAxisGroup.empty()) {
            xAxisGroup = this.svg.append('g')
                .attr('class', 'x-axis')
                .attr('transform',
                    `translate(0, ${this.marginTop + this.mainHeight})`);
        }
        xAxisGroup.call(xAxis
            .ticks(10, 's'))
    }


}

export default TaskChart;