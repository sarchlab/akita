import * as d3 from "d3";
class TaskView {
    constructor(yIndexAssigner, taskRenderer, xAxisDrawer) {
        this._yIndexAssigner = yIndexAssigner;
        this._taskRenderer = taskRenderer;
        this._xAxisDrawer = xAxisDrawer;
        this._canvas = null;
        this._tooltip = null;
        this._canvasWidth = 0;
        this._canvasHeight = 0;
        this._marginTop = 20;
        this._marginBottom = 20;
        this._marginLeft = 5;
        this._marginRight = 5;
        this._largeTaskHeight = 15;
        this._taskGroupGap = 10;
        this._startTime = 0;
        this._endTime = 0;
        this._xScale = null;
        this._toggleButton = null;
        this._onToggleCallback = null;
        this._dissectionView = null;
    }
    setToggleCallback(callback) {
        this._onToggleCallback = callback;
    }
    _createToggleButton() {
        if (this._toggleButton) {
            this._toggleButton.remove();
        }
        this._toggleButton = document.createElement('button');
        this._toggleButton.innerHTML = '🔍';
        this._toggleButton.className = 'task-dissection-toggle';
        this._toggleButton.title = 'Click this button to show task dissection view';
        this._toggleButton.style.cssText = `
      position: absolute;
      top: 10px;
      right: 50px;
      width: 35px;
      height: 35px;
      border: none;
      background: #f0f0f0;
      border-radius: 50%;
      cursor: pointer;
      font-size: 16px;
      display: flex;
      align-items: center;
      justify-content: center;
      box-shadow: 0 2px 4px rgba(0,0,0,0.2);
      z-index: 1000;
      transition: background-color 0.3s ease;
    `;
        this._toggleButton.addEventListener('mouseenter', () => {
            this._toggleButton.style.backgroundColor = '#e0e0e0';
        });
        this._toggleButton.addEventListener('mouseleave', () => {
            this._toggleButton.style.backgroundColor = '#f0f0f0';
        });
        this._toggleButton.addEventListener('click', () => {
            if (this._onToggleCallback) {
                this._onToggleCallback();
            }
        });
        this._canvas.style.position = 'relative';
        this._canvas.appendChild(this._toggleButton);
    }
    setCanvas(canvas, tooltip) {
        this._canvas = canvas;
        this._canvasWidth = this._canvas.offsetWidth;
        this._canvasHeight = this._canvas.offsetHeight;
        this._taskRenderer.setCanvas(canvas, tooltip);
        this._xAxisDrawer.setCanvas(canvas);
        d3.select(this._canvas)
            .select("svg")
            .attr("width", this._canvasWidth)
            .attr("height", this._canvasHeight);
        this.updateLayout();
        this._doRender();
        this._createToggleButton();
    }
    updateLayout() {
        this._canvasWidth = this._canvas.offsetWidth;
        this._canvasHeight = this._canvas.offsetHeight;
        d3.select(this._canvas)
            .select("svg")
            .attr("width", this._canvasWidth.toString())
            .attr("height", this._canvasHeight.toString());
        this._updateTimeScale();
        this._drawDivider();
    }
    _drawDivider() {
        const svg = d3.select(this._canvas).select("svg");
        let dividerGroup = svg.select(".divider");
        dividerGroup.remove();
        dividerGroup = svg.append("g").attr("class", "divider");
        dividerGroup
            .append("text")
            .attr("font-size", 20)
            .attr("fout-weight", "bold")
            .attr("x", 5)
            .attr("y", this._marginTop + this._taskGroupGap + 15)
            .attr("text-anchor", "left")
            .attr("style", "text-shadow: -1px -1px 0 #ffffff, 1px -1px 0 #ffffff, -1px 1px 0 #ffffff, 1px 1px 0 #ffffff")
            .text("Parent Task");
        dividerGroup
            .append("text")
            .attr("font-size", 20)
            .attr("fout-weight", "bold")
            .attr("x", 5)
            .attr("y", this._marginTop + this._taskGroupGap * 2 + this._largeTaskHeight + 16)
            .attr("text-anchor", "left")
            .attr("style", "text-shadow: -1px -1px 0 #ffffff, 1px -1px 0 #ffffff, -1px 1px 0 #ffffff, 1px 1px 0 #ffffff")
            .text("Current Task");
        dividerGroup
            .append("text")
            .attr("font-size", 20)
            .attr("fout-weight", "bold")
            .attr("x", 5)
            .attr("y", this._marginTop +
            this._taskGroupGap * 3 +
            this._largeTaskHeight * 2 +
            16)
            .attr("text-anchor", "left")
            .attr("style", "text-shadow: -1px -1px 0 #ffffff, 1px -1px 0 #ffffff, -1px 1px 0 #ffffff, 1px 1px 0 #ffffff; pointer-events:none; ")
            .text("Subtasks");
        const divider1Y = this._marginTop + this._taskGroupGap * 1.5 + this._largeTaskHeight;
        dividerGroup
            .append("line")
            .attr("x1", 0)
            .attr("x2", this._canvasWidth)
            .attr("y1", divider1Y)
            .attr("y2", divider1Y)
            .attr("stroke", "#000000")
            .attr("stroke-dasharray", 4);
        const divider2Y = this._marginTop + this._taskGroupGap * 2.5 + this._largeTaskHeight * 2;
        dividerGroup
            .append("line")
            .attr("x1", 0)
            .attr("x2", this._canvasWidth)
            .attr("y1", divider2Y)
            .attr("y2", divider2Y)
            .attr("stroke", "#000000")
            .attr("stroke-dasharray", 4);
    }
    showDissectionView() {
        this._createDissectionView();
    }
    hideDissectionView() {
        if (this._dissectionView) {
            // Clean up all existing tooltips
            const tooltips = document.querySelectorAll('.milestone-tooltip');
            tooltips.forEach(tooltip => {
                if (tooltip.parentNode) {
                    tooltip.parentNode.removeChild(tooltip);
                }
            });
            this._dissectionView.remove();
            this._dissectionView = null;
        }
    }
    _createDissectionView() {
        if (!this._task)
            return;
        if (this._dissectionView) {
            this._dissectionView.remove();
        }
        this._dissectionView = document.createElement('div');
        this._dissectionView.className = 'task-dissection-view';
        // Get left column width, don't block right column
        const leftColumnWidth = this._canvas.parentElement.offsetWidth;
        const leftColumn = this._canvas.parentElement;
        this._dissectionView.style.cssText = `
      position: fixed;
      top: ${leftColumn.getBoundingClientRect().top + 200}px;
      left: ${leftColumn.getBoundingClientRect().left}px;
      width: ${leftColumnWidth}px;
      height: ${leftColumn.offsetHeight - 200}px;
      background: white;
      padding: 20px;
      overflow-y: auto;
      border-top: 2px solid #ccc;
      z-index: 1000;
    `;
        // Parent Task section
        if (this._parentTask) {
            const parentSection = this._createTaskSection('Parent Task', this._parentTask, '#a5dee5', false);
            this._dissectionView.appendChild(parentSection);
        }
        // Current Task section (don't show red dots)
        const currentSection = this._createTaskSection('Current Task', this._task, '#e0f9b5', false);
        this._dissectionView.appendChild(currentSection);
        // Milestones section
        const milestonesSection = this._createMilestonesSection();
        this._dissectionView.appendChild(milestonesSection);
        // Sub Tasks section
        const subTasksSection = this._createSubTasksSection();
        this._dissectionView.appendChild(subTasksSection);
        // Add to body since we're using fixed positioning
        document.body.appendChild(this._dissectionView);
    }
    _createTaskSection(title, task, bgColor, showSteps = true) {
        const section = document.createElement('div');
        section.style.cssText = `
      margin-bottom: 20px;
    `;
        if (title) {
            const titleDiv = document.createElement('div');
            titleDiv.textContent = title;
            titleDiv.style.cssText = `
        font-size: 18px;
        font-weight: bold;
        margin-bottom: 10px;
        color: #333;
      `;
            section.appendChild(titleDiv);
        }
        // Create timeline container
        const timeAxisContainer = document.createElement('div');
        timeAxisContainer.style.cssText = `
      position: relative;
      height: 40px;
      margin-bottom: 8px;
      border: 1px solid #ddd;
      border-radius: 6px;
      background: #f9f9f9;
    `;
        // Calculate task bar position and width on timeline
        const containerWidth = this._canvas.parentElement.offsetWidth - 40; // minus padding
        const timeRange = this._endTime - this._startTime;
        const taskDuration = task.end_time - task.start_time;
        // Calculate position relative to timeline
        const leftOffset = ((task.start_time - this._startTime) / timeRange) * containerWidth;
        const barWidth = (taskDuration / timeRange) * containerWidth;
        const taskBar = document.createElement('div');
        taskBar.style.cssText = `
      position: absolute;
      left: ${Math.max(0, leftOffset)}px;
      top: 2px;
      width: ${Math.max(10, barWidth)}px;
      height: 36px;
      background: ${bgColor};
      border: 2px solid #ccc;
      border-radius: 4px;
    `;
        // Add task steps red dots (only show when showSteps is true)
        if (showSteps && task.steps && task.steps.length > 0) {
            task.steps.forEach(step => {
                const stepOffset = ((step.time - this._startTime) / timeRange) * containerWidth;
                if (stepOffset >= 0 && stepOffset <= containerWidth) {
                    const stepDot = document.createElement('div');
                    stepDot.style.cssText = `
            position: absolute;
            left: ${stepOffset - 3}px;
            top: 16px;
            width: 6px;
            height: 6px;
            background: #ff0000;
            border-radius: 50%;
            border: 1px solid #ffffff;
            cursor: pointer;
          `;
                    // Add tooltip for milestone dot
                    stepDot.addEventListener('mouseenter', (e) => {
                        this._showMilestoneInfo(step, 1);
                    });
                    stepDot.addEventListener('mouseleave', () => {
                        // Don't hide tooltip immediately, let it stay
                    });
                    timeAxisContainer.appendChild(stepDot);
                }
            });
        }
        timeAxisContainer.appendChild(taskBar);
        const taskInfo = document.createElement('div');
        taskInfo.textContent = `Type: ${task.kind}, What: ${task.what}, Location: ${task.location}, Time: ${task.start_time.toFixed(3)} to ${task.end_time.toFixed(3)}`;
        taskInfo.style.cssText = `
      font-size: 14px;
      color: #666;
      padding: 5px 0;
    `;
        section.appendChild(timeAxisContainer);
        section.appendChild(taskInfo);
        return section;
    }
    _createMilestonesSection() {
        const section = document.createElement('div');
        section.style.cssText = `
      margin-bottom: 20px;
    `;
        const titleDiv = document.createElement('div');
        titleDiv.textContent = 'Milestones Achieving';
        titleDiv.style.cssText = `
      font-size: 18px;
      font-weight: bold;
      margin-bottom: 10px;
      color: #333;
    `;
        // Create two containers: one for original timeline, one for stacked chart
        const originalContainer = document.createElement('div');
        originalContainer.style.cssText = `
      position: relative;
      height: 30px;
      border-top: 2px solid #666;
      border-bottom: 2px solid #666;
      background: #f0f0f0;
      margin-top: 25px;
      margin-bottom: 10px;
    `;
        const stackedContainer = document.createElement('div');
        stackedContainer.style.cssText = `
      position: relative;
      height: 60px;
      border: 1px solid #ccc;
      background: #f9f9f9;
      border-radius: 4px;
    `;
        // Add subtitle for stacked chart
        const stackedTitle = document.createElement('div');
        stackedTitle.textContent = 'Milestones by Kind (Stacked Bar Chart)';
        stackedTitle.style.cssText = `
      font-size: 14px;
      font-weight: bold;
      margin-bottom: 5px;
      color: #666;
    `;
        // 计算容器宽度，与task bars保持一致
        const containerWidth = this._canvas.parentElement.offsetWidth - 40; // minus padding
        const timeRange = this._endTime - this._startTime;
        // Create original milestone visualization
        this._createOriginalMilestoneVisualization(originalContainer, containerWidth, timeRange);
        // Create stacked milestone visualization  
        this._createStackedMilestoneVisualization(stackedContainer, containerWidth, timeRange);
        section.appendChild(titleDiv);
        section.appendChild(originalContainer);
        section.appendChild(stackedTitle);
        section.appendChild(stackedContainer);
        return section;
    }
    _createSubTasksSection() {
        const section = document.createElement('div');
        section.style.cssText = `
      margin-bottom: 20px;
    `;
        const titleDiv = document.createElement('div');
        titleDiv.textContent = 'Sub Tasks';
        titleDiv.style.cssText = `
      font-size: 18px;
      font-weight: bold;
      margin-bottom: 10px;
      color: #333;
    `;
        section.appendChild(titleDiv);
        if (this._subTasks && this._subTasks.length > 0) {
            this._subTasks.forEach(subTask => {
                const subTaskDiv = this._createTaskSection('', subTask, '#fefdca', false);
                subTaskDiv.style.marginBottom = '10px'; // remove left margin, keep alignment
                section.appendChild(subTaskDiv);
            });
        }
        else {
            const noSubTasksDiv = document.createElement('div');
            noSubTasksDiv.textContent = 'No sub tasks';
            noSubTasksDiv.style.cssText = `
        font-size: 14px;
        color: #666;
        font-style: italic;
      `;
            section.appendChild(noSubTasksDiv);
        }
        return section;
    }
    _getDarkerColor(hexColor) {
        // Convert color to darker color for gradient
        const colorMap = {
            '#FF6B6B': '#E74C3C',
            '#FFD93D': '#F1C40F',
            '#52C41A': '#27AE60',
            '#9B59B6': '#8E44AD',
            '#FF8C00': '#E67E22',
            '#1E90FF': '#3498DB',
            '#20B2AA': '#16A085', // light sea green -> darker teal
        };
        return colorMap[hexColor] || hexColor;
    }
    _groupMilestonesByTime(steps) {
        // Group milestones by time point, consistent with TaskRenderer logic
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
        return Object.values(groups).sort((a, b) => a.time - b.time);
    }
    _showMilestoneInfo(milestone, milestoneNumber) {
        // Show milestone info in right column, format consistent with red dot hover
        const tooltip = document.querySelector('.curr-task-info');
        if (tooltip) {
            tooltip.innerHTML = `<div style="text-align: left; min-width: 310px;">
            <h4>Milestone ${milestoneNumber} at ${this._smartString(milestone.time)}</h4>
            <div style="margin-bottom: 8px;">
                <span style="background-color: #ffeb3b; padding: 2px 4px; border-radius: 3px;">Kind:</span> ${milestone.kind || 'N/A'}<br/>
                <span style="background-color: #e3f2fd; padding: 2px 4px; border-radius: 3px;">What:</span> ${milestone.what || 'N/A'}
            </div>
        </div>`;
            tooltip.classList.add('showing');
        }
    }
    _showMilestoneGroupInfo(milestones, time) {
        // Show multiple milestone info in right column, format consistent with red dot hover
        const tooltip = document.querySelector('.curr-task-info');
        if (tooltip) {
            let content = `<div style="text-align: left; min-width: 310px;">
            <h4>Milestone${milestones.length > 1 ? 's' : ''} at ${this._smartString(time)}</h4>`;
            milestones.forEach((milestone, index) => {
                content += `<div style="margin-bottom: 8px;">
                <strong>Milestone${milestones.length > 1 ? ` ${index + 1}` : ''}:</strong><br/>
                <span style="background-color: #ffeb3b; padding: 2px 4px; border-radius: 3px;">Kind:</span> ${milestone.kind || 'N/A'}<br/>
                <span style="background-color: #e3f2fd; padding: 2px 4px; border-radius: 3px;">What:</span> ${milestone.what || 'N/A'}
            </div>`;
            });
            content += `</div>`;
            tooltip.innerHTML = content;
            tooltip.classList.add('showing');
        }
    }
    _smartString(value) {
        // Simple time formatting function, simulate smartString functionality
        if (value < 0.001) {
            return (value * 1000000).toFixed(2) + 'μs';
        }
        else if (value < 1) {
            return (value * 1000).toFixed(2) + 'ms';
        }
        else {
            return value.toFixed(2) + 's';
        }
    }
    _createStackedMilestoneVisualization(container, containerWidth, timeRange) {
        if (!this._task || !this._task.steps || this._task.steps.length === 0) {
            const noMilestones = document.createElement('div');
            noMilestones.textContent = 'No milestones available';
            noMilestones.style.cssText = `
        position: absolute;
        top: 50%;
        left: 50%;
        transform: translate(-50%, -50%);
        color: #666;
        font-style: italic;
      `;
            container.appendChild(noMilestones);
            return;
        }
        // Group milestones by kind
        const milestonesByKind = {};
        this._task.steps.forEach(step => {
            const kind = step.kind || 'unknown';
            if (!milestonesByKind[kind]) {
                milestonesByKind[kind] = [];
            }
            milestonesByKind[kind].push(step);
        });
        // Sort kinds and get colors
        const kinds = Object.keys(milestonesByKind).sort();
        const colors = ['#FF6B6B', '#FFD93D', '#52C41A', '#9B59B6', '#FF8C00', '#1E90FF', '#20B2AA', '#E74C3C', '#F39C12'];
        // Calculate time bins (similar to component view)
        const numBins = Math.min(50, containerWidth / 10); // Reasonable number of bars
        const binWidth = containerWidth / numBins;
        const binDuration = timeRange / numBins;
        for (let i = 0; i < numBins; i++) {
            const binStartTime = this._startTime + i * binDuration;
            const binEndTime = this._startTime + (i + 1) * binDuration;
            const binX = i * binWidth;
            // Count milestones by kind in this bin
            const kindCounts = {};
            kinds.forEach(kind => kindCounts[kind] = 0);
            // Check if task is active during this bin
            const taskActive = this._task.start_time <= binEndTime && this._task.end_time >= binStartTime;
            if (taskActive) {
                // Count milestones that occur in this bin
                this._task.steps.forEach(milestone => {
                    if (milestone.time >= binStartTime && milestone.time < binEndTime) {
                        kindCounts[milestone.kind]++;
                    }
                });
                // If no milestones in this bin but task is active, use the most recent milestone kind
                const totalMilestones = Object.values(kindCounts).reduce((sum, count) => sum + count, 0);
                if (totalMilestones === 0) {
                    // Find the most recent milestone before this bin
                    let latestMilestone = null;
                    let latestTime = -1;
                    this._task.steps.forEach(step => {
                        if (step.time <= binStartTime && step.time > latestTime) {
                            latestTime = step.time;
                            latestMilestone = step;
                        }
                    });
                    if (latestMilestone) {
                        kindCounts[latestMilestone.kind] = 1;
                    }
                }
            }
            // Draw stacked bar for this bin
            let currentY = container.offsetHeight;
            const maxCount = Math.max(...Object.values(kindCounts));
            const barHeight = container.offsetHeight * 0.8; // Use 80% of container height
            kinds.forEach((kind, kindIndex) => {
                const count = kindCounts[kind];
                if (count > 0) {
                    const segmentHeight = (count / Math.max(maxCount, 1)) * barHeight;
                    const segment = document.createElement('div');
                    segment.style.cssText = `
            position: absolute;
            left: ${binX}px;
            bottom: ${container.offsetHeight - currentY}px;
            width: ${binWidth - 1}px;
            height: ${segmentHeight}px;
            background: ${colors[kindIndex % colors.length]};
            border: 1px solid white;
            cursor: pointer;
            opacity: 0.8;
          `;
                    // Add tooltip
                    segment.addEventListener('mouseenter', (e) => {
                        this._showStackedTooltip(e, kind, count, binStartTime, binEndTime);
                    });
                    segment.addEventListener('mouseleave', () => {
                        this._hideStackedTooltip();
                    });
                    container.appendChild(segment);
                    currentY -= segmentHeight;
                }
            });
        }
        // Add legend
        this._createStackedLegend(container, kinds, colors);
    }
    _showStackedTooltip(event, kind, count, startTime, endTime) {
        const tooltip = document.querySelector('.curr-task-info');
        if (tooltip) {
            tooltip.innerHTML = `<div style="text-align: left; min-width: 250px;">
        <h4>Milestone Kind: ${kind}</h4>
        <div style="margin-bottom: 8px;">
          <span style="background-color: #ffeb3b; padding: 2px 4px; border-radius: 3px;">Count:</span> ${count}<br/>
          <span style="background-color: #e3f2fd; padding: 2px 4px; border-radius: 3px;">Time:</span> ${this._smartString(startTime)} - ${this._smartString(endTime)}
        </div>
      </div>`;
            tooltip.classList.add('showing');
        }
    }
    _hideStackedTooltip() {
        const tooltip = document.querySelector('.curr-task-info');
        if (tooltip) {
            tooltip.classList.remove('showing');
        }
    }
    _createOriginalMilestoneVisualization(container, containerWidth, timeRange) {
        // Use task steps as milestone points (these are the red dots in the upper view)
        if (this._task && this._task.steps && this._task.steps.length > 0) {
            const sortedSteps = [...this._task.steps].sort((a, b) => a.time - b.time);
            // Use rainbow color scheme for milestone achieving (no duplicate colors)
            const milestoneColors = ['#FF6B6B', '#FFD93D', '#52C41A', '#9B59B6', '#FF8C00', '#1E90FF', '#20B2AA'];
            // Group milestones by time point, same logic as red dots
            const milestoneGroups = this._groupMilestonesByTime(sortedSteps);
            console.log(`Milestone groups:`, milestoneGroups);
            // Entire milestone bar from parent task start (with small offset) to last milestone group
            let barStartTime;
            if (this._parentTask) {
                // Start slightly before parent task (5% of parent task duration earlier)
                const parentDuration = this._parentTask.end_time - this._parentTask.start_time;
                barStartTime = this._parentTask.start_time - 0.05 * parentDuration;
            }
            else {
                barStartTime = this._startTime; // fallback to timeline start if no parent task
            }
            const lastGroupTime = milestoneGroups[milestoneGroups.length - 1].time;
            // Calculate milestone bar start position and total width
            const milestoneBarStartX = ((barStartTime - this._startTime) / timeRange) * containerWidth;
            const milestoneBarEndX = ((lastGroupTime - this._startTime) / timeRange) * containerWidth;
            const milestoneBarWidth = milestoneBarEndX - milestoneBarStartX;
            console.log(`Milestone bar: start=${milestoneBarStartX}, end=${milestoneBarEndX}, width=${milestoneBarWidth}`);
            // Create milestone bar background
            const milestoneBackground = document.createElement('div');
            milestoneBackground.style.cssText = `
        position: absolute;
        left: ${Math.max(0, milestoneBarStartX)}px;
        top: 0;
        width: ${Math.max(10, milestoneBarWidth)}px;
        height: 100%;
        background: #e8f4f8;
      `;
            container.appendChild(milestoneBackground);
            // Create segment for each milestone group - 1st color block corresponds to 1st red dot
            for (let i = 0; i < milestoneGroups.length; i++) {
                let segmentStartTime, segmentEndTime;
                if (i === 0) {
                    // First segment: from bar start time (slightly before parent task) to first milestone group time
                    segmentStartTime = barStartTime; // slightly before parent task start
                    segmentEndTime = milestoneGroups[0].time;
                }
                else {
                    // Subsequent segments: from previous milestone group to current milestone group
                    segmentStartTime = milestoneGroups[i - 1].time;
                    segmentEndTime = milestoneGroups[i].time;
                }
                const segmentStartX = ((segmentStartTime - this._startTime) / timeRange) * containerWidth;
                const segmentEndX = ((segmentEndTime - this._startTime) / timeRange) * containerWidth;
                const segmentWidth = segmentEndX - segmentStartX;
                if (segmentWidth > 0) {
                    console.log(`Creating milestone segment ${i + 1}: start=${segmentStartX}, end=${segmentEndX}, width=${segmentWidth}`);
                    const segment = document.createElement('div');
                    // Use corresponding milestone color, enhance gradient contrast
                    const gradientColor = milestoneColors[i % milestoneColors.length];
                    const darkerColor = this._getDarkerColor(gradientColor);
                    segment.style.cssText = `
            position: absolute;
            left: ${segmentStartX}px;
            top: 0;
            width: ${segmentWidth}px;
            height: 100%;
            background: linear-gradient(to right, ${gradientColor}40, ${darkerColor});
            cursor: pointer;
            transition: opacity 0.2s ease;
          `;
                    // Add click effect for each segment to show milestone info
                    const currentGroup = milestoneGroups[i];
                    segment.addEventListener('click', (e) => {
                        // Show all milestone info for current group
                        this._showMilestoneGroupInfo(currentGroup.steps, currentGroup.time);
                    });
                    // Add hover effect to indicate clickable
                    segment.addEventListener('mouseenter', () => {
                        segment.style.opacity = '0.8';
                        segment.style.cursor = 'pointer';
                    });
                    segment.addEventListener('mouseleave', () => {
                        segment.style.opacity = '1';
                    });
                    container.appendChild(segment);
                }
            }
            // Add milestone divider lines based on groups
            milestoneGroups.forEach((group, index) => {
                const milestoneX = ((group.time - this._startTime) / timeRange) * containerWidth;
                const dividerLine = document.createElement('div');
                dividerLine.style.cssText = `
          position: absolute;
          left: ${milestoneX - 1}px;
          top: 0;
          width: 2px;
          height: 100%;
          background: #000;
          z-index: 10;
        `;
                container.appendChild(dividerLine);
                // Add red flag on the last milestone divider line
                if (index === milestoneGroups.length - 1) {
                    console.log(`Adding red flag at position: x=${milestoneX + 8}, index=${index}, total=${milestoneGroups.length}`);
                    const flag = document.createElement('div');
                    flag.innerHTML = '🚩';
                    flag.style.cssText = `
            position: absolute;
            left: ${milestoneX + 8}px;
            top: -25px;
            font-size: 20px;
            z-index: 15;
            line-height: 1;
            pointer-events: none;
            transform: translateX(-50%);
          `;
                    container.appendChild(flag);
                    console.log('Red flag added to milestone container');
                }
            });
        }
    }
    _createStackedLegend(container, kinds, colors) {
        const legend = document.createElement('div');
        legend.style.cssText = `
      position: absolute;
      top: -40px;
      right: 0;
      display: flex;
      gap: 15px;
      font-size: 11px;
    `;
        kinds.forEach((kind, index) => {
            const legendItem = document.createElement('div');
            legendItem.style.cssText = `
        display: flex;
        align-items: center;
        gap: 5px;
      `;
            const colorBox = document.createElement('div');
            colorBox.style.cssText = `
        width: 12px;
        height: 12px;
        background: ${colors[index % colors.length]};
        opacity: 0.8;
      `;
            const label = document.createElement('span');
            label.textContent = kind;
            label.style.color = '#333';
            legendItem.appendChild(colorBox);
            legendItem.appendChild(label);
            legend.appendChild(legendItem);
        });
        container.appendChild(legend);
    }
    setTimeAxis(startTime, endTime) {
        this._startTime = startTime;
        this._endTime = endTime;
        this._xAxisDrawer.setTimeRange(startTime, endTime);
        this._updateTimeScale();
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
            .renderCustom(5);
    }
    updateXAxis() {
        this._taskRenderer.updateXAxis();
    }
    highlight(task) {
        this._taskRenderer.hightlight(task);
    }
    render(task, subTasks, parentTask) {
        this._task = task;
        this._subTasks = subTasks;
        this._parentTask = parentTask;
        let tasks = [];
        if (parentTask != null) {
            parentTask.isParentTask = true;
            tasks.push(parentTask);
        }
        task.isMainTask = true;
        tasks.push(task);
        tasks = tasks.concat(subTasks);
        this._allTasks = tasks;
        this._maxY = this._yIndexAssigner.assign(subTasks);
        this._doRender();
    }
    _doRender() {
        if (!this._allTasks) {
            return;
        }
        const tasks = this._allTasks;
        const barRegionHeight = this._canvasHeight - this._marginBottom - this._marginTop;
        const nonSubTaskRegionHeight = this._taskGroupGap * 4 + this._largeTaskHeight * 2;
        const subTaskRegionHeight = barRegionHeight - nonSubTaskRegionHeight;
        let barHeight = subTaskRegionHeight / this._maxY;
        if (barHeight > 10) {
            barHeight = 10;
        }
        this._taskRenderer
            .renderWithHeight((task) => {
            if (task.isParentTask) {
                return this._largeTaskHeight;
            }
            else if (task.isMainTask) {
                return this._largeTaskHeight;
            }
            else {
                return barHeight * 0.75;
            }
        })
            .renderWithY((task) => {
            if (task.isParentTask) {
                let extraHeight = this._taskGroupGap;
                return extraHeight + this._marginTop;
            }
            else if (task.isMainTask) {
                let extraHeight = this._taskGroupGap * 2 + this._largeTaskHeight;
                return extraHeight + this._marginTop;
            }
            else {
                let extraHeight = this._taskGroupGap * 3 + this._largeTaskHeight * 2;
                return task.yIndex * barHeight + extraHeight + this._marginTop;
            }
        })
            .render(tasks);
    }
}
export default TaskView;
