import * as d3 from 'd3'
import TaskColorCoder from './taskcolorcoder';
import TaskPage from './taskpage';
import {
    smartString
} from './smartvalue'

class TaskRenderer {

    /**
     * @param {TaskPage} taskPage
     * @param {TaskColorCoder} colorCoder
     */
    constructor(taskPage, colorCoder) {
        this._detailPage = taskPage;
        this._colorCoder = colorCoder;
        this._x = null;
        this._y = null;
        this._height = null;
        this._width = null;
    }

    setCanvas(canvas, tooltip) {
        this._canvas = canvas;
        this._tooltip = tooltip;
        return this
    }

    setXScale(xScale) {
        this._xScale = xScale;
        return this
    }

    updateXAxis() {
        const svg = d3.select(this._canvas)
            .select('svg');
        let taskBarGroup = svg.select('.task-bar');
        if (taskBarGroup.empty()) {
            return
        }

        const tr = d3.transition("xAxisUpdate")
            .duration(50)
            .ease(d3.easeCubic);

        taskBarGroup
            .selectAll('rect')
            // .transition(tr)
            .attr('x', (d) => {
                return this._xScale(d.start_time)
            })
            .attr('width', d => {
                return this._xScale(d.end_time) -
                    this._xScale(d.start_time);
            });

        return this
    }

    hightlight(task) {
        const svg = d3.select(this._canvas)
            .select('svg');

        let taskBarGroup = svg.select('.task-bar');
        if (taskBarGroup.empty()) {
            return
        }

        if (!task) {
            const tran = d3.transition("fade")
                .duration(1000)
                .ease(d3.easeCubic);
            taskBarGroup
                .selectAll('rect')
                // .transition(tran)
                .attr('opacity', 1.0)
                .attr('stroke-opacity', 0.2);
            return this
        } else {
            taskBarGroup
                .selectAll('rect')
                .attr('opacity', 0.4)
                .attr('stroke-opacity', 0.2);
        }

        if (typeof task === "object") {
            const t = d3.transition("hightlight")
                .duration(50)
                .ease(d3.easeCubic);
            taskBarGroup
                .select(`#task-${this._taskIdTag(task)}`)
                // .transition(t)
                .attr('opacity', 1.0)
                .attr('stroke-opacity', 0.8);
            return this
        }

        if (typeof task === "function") {
            const t = d3.transition("hightlight")
                .duration(50)
                .ease(d3.easeCubic);
            taskBarGroup
                .selectAll(`rect`)
                .filter((d) => {
                    return task(d)
                })
                // .transition(t)
                .attr('opacity', 1.0)
                .attr('stroke-opacity', 0.8);
            return this
        }

        return this
    }

    _taskIdTag(task) {
        return task.id
            .replaceAll("@", "-")
            .replaceAll('.', '-')
            .replaceAll('[', '-')
            .replaceAll(']', '-')
            .replaceAll('_', '-');
    }

    renderWithX(func) {
        this._x = func
        return this
    }

    _getXValue(task, i) {
        if (!this._x) {
            return this._xScale(task.start_time)
        }

        if (typeof this._x === "function") {
            return this._x(task, i)
        }

        return this._x
    }

    renderWithY(func) {
        this._y = func
        return this
    }

    _defaultYFunc(task) {
        return task.yIndex * 15;
    }

    _getYValue(task, i) {
        if (!this._y) {
            this._defaultYFunc(task)
        }

        if (typeof this._y === "function") {
            return this._y(task, i);
        }

        return this._y;
    }

    renderWithWidth(func) {
        this._width = func
        return this
    }

    _getWidthValue(task, i) {
        if (!this._width) {
            return this._xScale(task.end_time) - this._xScale(task.start_time);
        }

        if (typeof this._width === "function") {
            return this._width(task, i);
        }

        return this._width;
    }

    renderWithHeight(func) {
        this._height = func
        return this
    }

    _getHeightValue(task, i) {
        if (!this._height) {
            return 10;
        }

        if (typeof this._height === "function") {
            return this._height(task, i);
        }

        return 10;
    }

    /**
     * 
     * @param {Array[Task]} tasks 
     */
    render(tasks) {
        const svg = d3.select(this._canvas)
            .select('svg');

        let taskBarGroup = svg.select('.task-bar');
        if (taskBarGroup.empty()) {
            taskBarGroup = svg
                .append('g')
                .attr('class', 'task-bar')
                .lower();
        }


        const taskBars = taskBarGroup
            .selectAll('rect')
            .data(tasks, (d) => d.id);

        const taskBarsEnter = taskBars
            .enter()
            .append('rect')
            .attr('id', d => `task-${this._taskIdTag(d)}`)
            .attr('y', (d, i) => this._getYValue(d, i))
            .attr('height', (d, i) => this._getHeightValue(d, i))
            .attr('stroke', 'none')
            .attr('height', 0);

        taskBarsEnter
            .on("mouseover", (_, d) => {
                this._showTooltip(d)
                this._detailPage.highlight(d)
                console.log(d)
            })
            .on("mouseout", d => {
                this._hideTooltip()
                this._detailPage.highlight(null)
            });

        let dragging = false;
        let dragMoved = false
        let dragStartX = 0;
        let dragStartY = 0;
        let mouseUpTimeout = null;
        taskBarsEnter
            .on('mousedown', (event) => {
                dragging = true
                dragStartX = event.clientX
                dragStartY = event.clientY
                clearTimeout(mouseUpTimeout)
            })
            .on('mousemove', (event) => {
                if (dragging) {
                    if ((event.clientX - dragStartX) > 1 ||
                        (event.clientX - dragStartX) < -1 ||
                        (event.clientY - dragStartY) > 1 ||
                        (event.clientY - dragStartY) < -1
                    ) {
                        dragMoved = true;
                    }
                }
            })
            .on('click', (event, d) => {
                event.preventDefault();
                if (dragMoved) {
                    return
                }
                window.history.pushState(null, null, `/task?id=${d.id}`)
                this._detailPage.showTask(d);
            })
            .on('mouseup', () => {
                mouseUpTimeout = setTimeout(() => {
                    dragging = false;
                    dragMoved = false;
                }, 500)
            })

        const t = d3.transition("enter")
            .duration(300)
            .ease(d3.easeCubic);

        taskBarsEnter.merge(taskBars)
            .transition(t)
            .attr('x', (d, i) => this._getXValue(d, i))
            .attr('y', (d, i) => this._getYValue(d, i))
            .attr('width', (d, i) => this._getWidthValue(d, i))
            .attr('height', (d, i) => this._getHeightValue(d, i))
            .attr('fill', d => {
                return this._colorCoder.lookup(d)
            })
            .attr('stroke', '#000000')
            .attr('stroke-opacity', 0.2);

        taskBars.exit().remove();


        // Only render milestones for the main task (current selected task)
        tasks.forEach(task => {
            console.log("Task debug:", {
                id: task.id,
                steps: task.steps,
                isMainTask: task.isMainTask,
                hasSteps: task.steps && task.steps.length > 0
            });
            
            if (task.isMainTask && task.steps && task.steps.length > 0) {
                console.log("Rendering milestones for task:", task.id, "count:", task.steps.length);
                
                // Group milestones by time
                const milestoneGroups = this._groupMilestonesByTime(task.steps);
                console.log("Milestone groups:", milestoneGroups);
                
                const milestones = taskBarGroup
                    .selectAll(`.milestone-${this._taskIdTag(task)}`)
                    .data(milestoneGroups, d => `${task.id}-${d.time}`);

                const milestonesEnter = milestones
                    .enter()
                    .append('circle')
                    .attr('class', `milestone-${this._taskIdTag(task)}`)
                    .attr('r', d => d.steps.length > 1 ? 3 : 2) // Larger circle for multiple milestones
                    .attr('fill', 'red')
                    .attr('stroke', d => d.steps.length > 1 ? '#fff' : 'none') // White border for multiple
                    .attr('stroke-width', d => d.steps.length > 1 ? 1 : 0)
                    .attr('cy', (d) => this._getYValue(task) + this._getHeightValue(task) / 2);

                milestonesEnter
                    .on("mouseover", (event, d) => {
                        this._showMilestoneGroupTooltip(d, event);
                    })
                    .on("mouseout", () => {
                        this._hideTooltip();
                    });

                milestonesEnter.merge(milestones)
                    .transition(t)
                    .attr('cx', d => {
                        console.log("Milestone position calculation:", {
                            time: d.time,
                            xPos: this._xScale(d.time),
                            count: d.steps.length
                        });
                        return this._xScale(d.time);
                    })
                    .attr('r', d => d.steps.length > 1 ? 3 : 2)
                    .attr('stroke', d => d.steps.length > 1 ? '#fff' : 'none')
                    .attr('stroke-width', d => d.steps.length > 1 ? 1 : 0);

                milestones.exit().remove();
            }
        });
        return this
    }

    _groupMilestonesByTime(steps) {
        const groups = {};
        steps.forEach(step => {
            const timeKey = step.time.toString();
            if (!groups[timeKey]) {
                groups[timeKey] = {
                    time: step.time,
                    steps: []
                };
            }
            groups[timeKey].steps.push(step);
        });
        return Object.values(groups);
    }

    _showMilestoneGroupTooltip(milestoneGroup, event) {
        const steps = milestoneGroup.steps;
        
        let content = `<div style="text-align: left; min-width: 310px;">
            <h4>Milestone${steps.length > 1 ? 's' : ''} at ${smartString(milestoneGroup.time)}</h4>`;
            
        steps.forEach((step, index) => {
            content += `<div style="margin-bottom: 8px;">
                <strong>Milestone${steps.length > 1 ? ` ${index + 1}` : ''}:</strong><br/>
                <span style="background-color: #ffeb3b; padding: 2px 4px; border-radius: 3px;">Kind:</span> ${step.kind || 'N/A'}<br/>
                <span style="background-color: #e3f2fd; padding: 2px 4px; border-radius: 3px;">What:</span> ${step.what || 'N/A'}
            </div>`;
        });
        
        content += `</div>`;
        this._tooltip.innerHTML = content;

        this._tooltip.classList.add('showing');
        
        const tooltipWidth = this._tooltip.offsetWidth;
        const tooltipHeight = this._tooltip.offsetHeight;
        const x = event.pageX - tooltipWidth / 2;
        const y = event.pageY - tooltipHeight - 10;
        
        this._tooltip.style.left = `${x}px`;
        this._tooltip.style.top = `${y}px`;
    }

    _showTooltip(task) {
        const tableLeftCol = 3;
        const tableRightcol = 12 - tableLeftCol;

        this._tooltip.innerHTML = `
<div class="container">
    <div class="row">
        <h4> ${task.kind} - ${task.what} </h4>
    </div>
    <dl class="row">
        <dt class="col-sm-${tableLeftCol}">ID</dt>
        <dd class="col-sm-${tableRightcol}">${task.id}</dd>

        <dt class="col-sm-${tableLeftCol}">Kind</dt>
        <dd class="col-sm-${tableRightcol}">${task.kind}</dd>

        <dt class="col-sm-${tableLeftCol}">What</dt>
        <dd class="col-sm-${tableRightcol}">${task.what}</dd>

        <dt class="col-sm-${tableLeftCol}">Where</dt>
        <dd class="col-sm-${tableRightcol}">${task.where}</dd>

        <dt class="col-sm-${tableLeftCol}">Start</dt>
        <dd class="col-sm-${tableRightcol}">
            ${smartString(task['start_time'])}
        </dd>

        <dt class="col-sm-${tableLeftCol}">End</dt>
        <dd class="col-sm-${tableRightcol}">
            ${smartString(task['end_time'])}
        </dd>

        <dt class="col-sm-${tableLeftCol}">Duration</dt>
        <dd class="col-sm-${tableRightcol}">
            ${smartString(task['end_time'] - task['start_time'])}
        </dd>
    </dl>
</div>`

        this._tooltip.classList.add('showing');
    }

    _hideTooltip() {
        this._tooltip.classList.remove('showing');
    }

    _showMilestoneTooltip(step, event) {
        const tableLeftCol = 3;
        const tableRightCol = 12 - tableLeftCol;

        this._tooltip.innerHTML = `
        <div class="container">
            <div class="row">
                <h4>Milestone</h4>
            </div>
            <dl class="row">
                <dt class="col-sm-${tableLeftCol}">Time</dt>
                <dd class="col-sm-${tableRightCol}">${smartString(step.time)}</dd>

                <dt class="col-sm-${tableLeftCol}">Kind</dt>
                <dd class="col-sm-${tableRightCol}">${step.kind || 'N/A'}</dd>

                <dt class="col-sm-${tableLeftCol}">What</dt>
                <dd class="col-sm-${tableRightCol}">${step.what || 'N/A'}</dd>
            </dl>
        </div>`;

        this._tooltip.classList.add('showing');
        
        const tooltipWidth = this._tooltip.offsetWidth;
        const tooltipHeight = this._tooltip.offsetHeight;
        const x = event.pageX - tooltipWidth / 2;
        const y = event.pageY - tooltipHeight - 10;
        
        this._tooltip.style.left = `${x}px`;
        this._tooltip.style.top = `${y}px`;
    }
}

export default TaskRenderer;