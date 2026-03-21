import * as d3 from 'd3'

class IPSChart {
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

        this.renderBarGroup(this.data.StartCount,
            'start-bar-group', 'green', 0);
        this.renderBarGroup(this.data.CompleteCount,
            'complete-bar-group', 'red', 0.35);

        this.renderAxis()

    }

    calculateDimension() {
        this.canvasWidth = this.canvas.offsetWidth;
        this.canvasHeight = this.canvas.offsetHeight;
        this.barWidth = this.canvasWidth / this.data.CompleteCount.length;
        this.marginLeft = 40;
        this.marginRight = 5;
        this.marginTop = 5;
        this.marginBottom = 50;
        this.barRegionHeight = this.canvasHeight
            - this.marginBottom
            - this.marginTop;
        this.barRegionWidth = this.canvasWidth
            - this.marginLeft
            - this.marginRight;
    }

    calculateScale() {
        const maxCompleteCount = Math.max(...this.data.CompleteCount);
        const maxStartCount = Math.max(...this.data.StartCount);
        this.maxCount = Math.max(maxCompleteCount, maxStartCount);

        this.yScale = d3.scaleLinear()
            .domain([0, this.maxCount])
            .range([this.barRegionHeight, this.marginTop]);

        this.xScale = d3.scaleLinear()
            .domain([0, this.data.CompleteCount.length])
            .range([this.marginLeft, this.marginLeft + this.barRegionWidth]);

        this.xTimeScale = d3.scaleLinear()
            .domain([this.data.Start, this.data.End])
            .range([this.marginLeft, this.marginLeft + this.barRegionWidth]);
    }

    renderBarGroup(data, groupName, color, xOffset) {
        let startBarGroup = this.svg.select(`.${groupName}`);
        if (startBarGroup.empty()) {
            startBarGroup =
                this.svg.append('g').attr('class', groupName);
        }

        const startBars = startBarGroup
            .selectAll('rect')
            .data(data);
        const startBarsEnter = startBars
            .enter()
            .append('rect')
            .attr('x', (d, i) => this.xScale(i + xOffset))
            .attr('y', (d) => (this.barRegionHeight + this.marginTop))
            .attr('width', this.barWidth * 0.3)
            .attr('height', 0)
            .attr('fill', color);
        startBarsEnter.merge(startBars)
            .transition()
            .attr('y', (d) => this.yScale(d) + this.marginTop)
            .attr('height', (d) =>
                this.barRegionHeight - this.yScale(d)
            );
    }

    renderAxis() {
        const yAxis = d3.axisLeft(this.yScale);
        let yAxisGroup = this.svg.select('y-axis');
        if (yAxisGroup.empty()) {
            yAxisGroup = this.svg.append('g')
                .attr('class', 'y-axis')
                .attr('transform', `translate(${this.marginLeft}, ${this.marginTop})`);
        }
        yAxisGroup.call(yAxis
            .ticks(6, 's'));

        const xAxis = d3.axisBottom(this.xTimeScale);
        let xAxisGroup = this.svg.select('x-axis');
        if (xAxisGroup.empty()) {
            xAxisGroup = this.svg.append('g')
                .attr('class', 'x-axis')
                .attr('transform',
                    `translate(0, ${this.marginTop + this.barRegionHeight})`);
        }
        xAxisGroup.call(xAxis
            .ticks(10, 's'))
    }
}

export default IPSChart
