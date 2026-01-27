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
                // Only show hover tooltip if no task is locked
                if (!this._detailPage.isTaskLocked) {
                    this._showTooltip(d)
                    this._detailPage.highlight(d)
                }
                console.log(d)
            })
            .on("mouseout", d => {
                // Only hide tooltip if no task is locked
                if (!this._detailPage.isTaskLocked) {
                    this._hideTooltip()
                    this._detailPage.highlight(null)
                }
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
                this._detailPage.handleTaskClick(d);
            })
            .on('dblclick', (event, d) => {
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

                // Sort milestone groups by time for connecting lines
                const sortedMilestoneGroups = milestoneGroups.sort((a, b) => a.time - b.time);

                // Detect collisions and mark overlapping milestones
                const minPixelDistance = 10; // Minimum pixel distance between circles
                sortedMilestoneGroups.forEach((group, index) => {
                    group.xOffset = 0; // Default: no x offset
                    group.isOverlapping = false; // Default: not overlapping

                    if (index > 0) {
                        const prevGroup = sortedMilestoneGroups[index - 1];
                        const prevX = this._xScale(prevGroup.time);
                        const currX = this._xScale(group.time);
                        const pixelDistance = Math.abs(currX - prevX);

                        // If circles are too close, mark as overlapping and apply offset
                        if (pixelDistance < minPixelDistance) {
                            group.isOverlapping = true;
                            prevGroup.isOverlapping = true;
                            // Offset by 1-2 pixels to slightly separate them
                            group.xOffset = 2;
                        }
                    }
                });
                
                // Render connecting wavy lines between milestones
                if (sortedMilestoneGroups.length > 1) {
                    const taskY = this._getYValue(task) + this._getHeightValue(task) / 2;
                    
                    // Remove old wavy lines
                    taskBarGroup.selectAll(`.milestone-line-${this._taskIdTag(task)}`).remove();
                    
                    for (let i = 0; i < sortedMilestoneGroups.length - 1; i++) {
                        const currentMilestone = sortedMilestoneGroups[i];
                        const nextMilestone = sortedMilestoneGroups[i + 1];
                        
                        const x1 = this._xScale(currentMilestone.time);
                        const x2 = this._xScale(nextMilestone.time);
                        const distance = x2 - x1;
                        
                        // Only draw line if milestones are far enough apart
                        if (distance > 10) {
                            const pathData = this._createWavyPath(x1, x2, taskY, distance);
                            
                            taskBarGroup
                                .append('path')
                                .attr('class', `milestone-line-${this._taskIdTag(task)}`)
                                .attr('d', pathData)
                                .attr('stroke', 'red')
                                .attr('stroke-width', 1.5)
                                .attr('fill', 'none')
                                .attr('opacity', 0.7)
                                .style('pointer-events', 'none'); // Don't interfere with mouse events
                        }
                    }
                }
                
                const milestones = taskBarGroup
                    .selectAll(`.milestone-${this._taskIdTag(task)}`)
                    .data(milestoneGroups, d => `${task.id}-${d.time}`);

                const milestonesEnter = milestones
                    .enter()
                    .append('circle')
                    .attr('class', `milestone-${this._taskIdTag(task)}`)
                    .attr('r', 3)
                    .attr('fill', 'red')
                    .attr('stroke', d => {
                        // Add white stroke for overlapping milestones
                        if (d.isOverlapping) {
                            return '#fff';
                        }
                        return d.steps.length > 1 ? '#fff' : 'none';
                    })
                    .attr('stroke-width', d => {
                        // Thicker stroke for overlapping milestones
                        if (d.isOverlapping) {
                            return 2;
                        }
                        return d.steps.length > 1 ? 1 : 0;
                    })
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
                        const baseX = this._xScale(d.time);
                        const offsetX = baseX + (d.xOffset || 0);
                        console.log("Milestone position calculation:", {
                            time: d.time,
                            xPos: baseX,
                            xOffset: d.xOffset || 0,
                            finalX: offsetX,
                            count: d.steps.length,
                            isOverlapping: d.isOverlapping || false
                        });
                        return offsetX;
                    })
                    .attr('cy', (d) => this._getYValue(task) + this._getHeightValue(task) / 2)
                    .attr('r', 3)
                    .attr('stroke', d => {
                        // Add white stroke for overlapping milestones
                        if (d.isOverlapping) {
                            return '#fff';
                        }
                        return d.steps.length > 1 ? '#fff' : 'none';
                    })
                    .attr('stroke-width', d => {
                        // Thicker stroke for overlapping milestones
                        if (d.isOverlapping) {
                            return 2;
                        }
                        return d.steps.length > 1 ? 1 : 0;
                    });

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

    /**
     * Creates a wavy path between two points
     * @param {number} x1 - Start x coordinate
     * @param {number} x2 - End x coordinate  
     * @param {number} y - Y coordinate (same for both points)
     * @param {number} distance - Distance between points
     * @returns {string} SVG path data string
     */
    _createWavyPath(x1, x2, y, distance) {
        // Calculate wave parameters based on distance
        const waveLength = Math.min(distance / 6, 12);
        const amplitude = 4;
        const numWaves = Math.max(2, Math.floor(distance / waveLength));
        
        let pathData = `M ${x1} ${y}`;
        
        // Create smooth wavy line using quadratic curves
        for (let i = 0; i < numWaves; i++) {
            const waveProgress = i / numWaves;
            const nextWaveProgress = (i + 1) / numWaves;
            
            const currentX = x1 + (waveProgress * distance);
            const nextX = x1 + (nextWaveProgress * distance);
            const midX = (currentX + nextX) / 2;
            
            // Alternate wave direction for each wave
            const waveDirection = (i % 2 === 0) ? 1 : -1;
            const controlY = y + (amplitude * waveDirection);
            
            // Add quadratic curve to create wave
            pathData += ` Q ${midX} ${controlY} ${nextX} ${y}`;
        }
        
        return pathData;
    }
}

export default TaskRenderer;