import * as d3 from "d3";
import TaskYIndexAssigner from "./taskyindexassigner";
import TaskRenderer from "./taskrenderer";
import XAxisDrawer from "./xaxisdrawer";
import { Task, TaskMilestone, TaskStep } from "./task";

class TaskView {
  private _yIndexAssigner: TaskYIndexAssigner;
  private _taskRenderer: TaskRenderer;
  private _xAxisDrawer: XAxisDrawer;
  private _canvas: HTMLElement;
  private _tooltip: HTMLElement;
  private _canvasWidth: number;
  private _canvasHeight: number;
  private _marginTop: number;
  private _marginBottom: number;
  private _marginLeft: number;
  private _marginRight: number;
  private _startTime: number;
  private _endTime: number;
  private _xScale: d3.ScaleLinear<number, number>;
  private _task: Task;
  private _parentTask: Task;
  private _subTasks: Array<Task>;
  private _allTasks: Array<Task>;
  private _maxY: number;
  private _largeTaskHeight: number;
  private _taskGroupGap: number;
  private _toggleButton: HTMLElement;
  private _onToggleCallback: (() => void) | null;
  private _dissectionView: HTMLElement;

  constructor(
    yIndexAssigner: TaskYIndexAssigner,
    taskRenderer: TaskRenderer,
    xAxisDrawer: XAxisDrawer
  ) {
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

  setToggleCallback(callback: () => void) {
    this._onToggleCallback = callback;
  }

  private _createToggleButton() {
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


  setCanvas(canvas: HTMLElement, tooltip: HTMLElement) {
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

  private _drawDivider() {
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
      .attr(
        "style",
        "text-shadow: -1px -1px 0 #ffffff, 1px -1px 0 #ffffff, -1px 1px 0 #ffffff, 1px 1px 0 #ffffff"
      )
      .text("Parent Task");

    dividerGroup
      .append("text")
      .attr("font-size", 20)
      .attr("fout-weight", "bold")
      .attr("x", 5)
      .attr(
        "y",
        this._marginTop + this._taskGroupGap * 2 + this._largeTaskHeight + 16
      )
      .attr("text-anchor", "left")
      .attr(
        "style",
        "text-shadow: -1px -1px 0 #ffffff, 1px -1px 0 #ffffff, -1px 1px 0 #ffffff, 1px 1px 0 #ffffff"
      )
      .text("Current Task");

    dividerGroup
      .append("text")
      .attr("font-size", 20)
      .attr("fout-weight", "bold")
      .attr("x", 5)
      .attr(
        "y",
        this._marginTop +
          this._taskGroupGap * 3 +
          this._largeTaskHeight * 2 +
          16
      )
      .attr("text-anchor", "left")
      .attr(
        "style",
        "text-shadow: -1px -1px 0 #ffffff, 1px -1px 0 #ffffff, -1px 1px 0 #ffffff, 1px 1px 0 #ffffff; pointer-events:none; "
      )
      .text("Subtasks");

    const divider1Y =
      this._marginTop + this._taskGroupGap * 1.5 + this._largeTaskHeight;
    dividerGroup
      .append("line")
      .attr("x1", 0)
      .attr("x2", this._canvasWidth)
      .attr("y1", divider1Y)
      .attr("y2", divider1Y)
      .attr("stroke", "#000000")
      .attr("stroke-dasharray", 4);

    const divider2Y =
      this._marginTop + this._taskGroupGap * 2.5 + this._largeTaskHeight * 2;
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

  private _createDissectionView() {
    if (!this._task) return;

    if (this._dissectionView) {
      this._dissectionView.remove();
    }

    this._dissectionView = document.createElement('div');
    this._dissectionView.className = 'task-dissection-view';
    // Get left column width, don't block right column
    const leftColumnWidth = this._canvas.parentElement.offsetWidth;
    
    this._dissectionView.style.cssText = `
      position: absolute;
      top: 200px;
      left: 0;
      width: ${leftColumnWidth}px;
      bottom: 0;
      background: white;
      padding: 20px;
      overflow-y: auto;
      border-top: 2px solid #ccc;
    `;

    // Parent Task section
    if (this._parentTask) {
      const parentSection = this._createTaskSection('Parent Task', this._parentTask, '#a5dee5', true);
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

    // Add to container
    this._canvas.parentElement.appendChild(this._dissectionView);
  }

  private _createTaskSection(title: string, task: Task, bgColor: string, showSteps: boolean = true): HTMLElement {
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
          `;
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

  private _createMilestonesSection(): HTMLElement {
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

    // Create timeline container
    const milestoneContainer = document.createElement('div');
    milestoneContainer.style.cssText = `
      position: relative;
      height: 30px;
      border-top: 2px solid #666;
      border-bottom: 2px solid #666;
      background: #f0f0f0;
      margin-top: 25px;
    `;

    // 计算容器宽度，与task bars保持一致
    const containerWidth = this._canvas.parentElement.offsetWidth - 40; // minus padding
    const timeRange = this._endTime - this._startTime;
    
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
      } else {
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
      milestoneContainer.appendChild(milestoneBackground);
      
      // Create segment for each milestone group - 1st color block corresponds to 1st red dot
      for (let i = 0; i < milestoneGroups.length; i++) {
        let segmentStartTime, segmentEndTime;
        
        if (i === 0) {
          // First segment: from bar start time (slightly before parent task) to first milestone group time
          segmentStartTime = barStartTime; // slightly before parent task start
          segmentEndTime = milestoneGroups[0].time;
        } else {
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
          
          milestoneContainer.appendChild(segment);
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
        milestoneContainer.appendChild(dividerLine);
        
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
          milestoneContainer.appendChild(flag);
          console.log('Red flag added to milestone container');
        }
      });
    }

    section.appendChild(titleDiv);
    section.appendChild(milestoneContainer);

    return section;
  }

  private _createSubTasksSection(): HTMLElement {
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
        const subTaskDiv = this._createTaskSection('', subTask, '#fefdca', true);
        subTaskDiv.style.marginBottom = '10px'; // remove left margin, keep alignment
        section.appendChild(subTaskDiv);
      });
    } else {
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

  private _getDarkerColor(hexColor: string): string {
    // Convert color to darker color for gradient
    const colorMap: { [key: string]: string } = {
      '#FF6B6B': '#E74C3C', // coral red -> darker red
      '#FFD93D': '#F1C40F', // bright yellow -> darker yellow
      '#52C41A': '#27AE60', // green -> darker green
      '#9B59B6': '#8E44AD', // purple -> darker purple
      '#FF8C00': '#E67E22', // dark orange -> darker orange
      '#1E90FF': '#3498DB', // dodger blue -> blue
      '#20B2AA': '#16A085', // light sea green -> darker teal
    };
    return colorMap[hexColor] || hexColor;
  }

  private _groupMilestonesByTime(steps: TaskStep[]): { time: number; steps: TaskStep[] }[] {
    // Group milestones by time point, consistent with TaskRenderer logic
    const groups: { [key: string]: { time: number; steps: TaskStep[] } } = {};
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

  private _showMilestoneInfo(milestone: TaskStep, milestoneNumber: number) {
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

  private _showMilestoneGroupInfo(milestones: TaskStep[], time: number) {
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

  private _smartString(value: number): string {
    // Simple time formatting function, simulate smartString functionality
    if (value < 0.001) {
      return (value * 1000000).toFixed(2) + 'μs';
    } else if (value < 1) {
      return (value * 1000).toFixed(2) + 'ms';
    } else {
      return value.toFixed(2) + 's';
    }
  }

  setTimeAxis(startTime: number, endTime: number) {
    this._startTime = startTime;
    this._endTime = endTime;
    this._xAxisDrawer.setTimeRange(startTime, endTime);
    this._updateTimeScale();
  }

  private _updateTimeScale() {
    this._xScale = d3
      .scaleLinear()
      .domain([this._startTime, this._endTime])
      .range([this._marginLeft, this._canvasWidth - this._marginLeft]);

    this._taskRenderer.setXScale(this._xScale);
    this._drawXAxis();
  }

  private _drawXAxis() {
    this._xAxisDrawer
    .setCanvasHeight(this._canvasHeight)
    .setCanvasWidth(this._canvasWidth)
    .setScale(this._xScale)
    .renderCustom(5); 
  }

  updateXAxis() {
    this._taskRenderer.updateXAxis();
  }

  highlight(task: Task | ((t: Task) => boolean)) {
    this._taskRenderer.hightlight(task);
  }

  render(task: Task, subTasks: Array<Task>, parentTask: Task) {
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
    const barRegionHeight =
      this._canvasHeight - this._marginBottom - this._marginTop;
    const nonSubTaskRegionHeight =
      this._taskGroupGap * 4 + this._largeTaskHeight * 2;
    const subTaskRegionHeight = barRegionHeight - nonSubTaskRegionHeight;
    let barHeight = subTaskRegionHeight / this._maxY;
    if (barHeight > 10) {
      barHeight = 10;
    }

    this._taskRenderer
      .renderWithHeight((task: Task) => {
        if (task.isParentTask) {
          return this._largeTaskHeight;
        } else if (task.isMainTask) {
          return this._largeTaskHeight;
        } else {
          return barHeight * 0.75;
        }
      })
      .renderWithY((task: Task) => {
        if (task.isParentTask) {
          let extraHeight = this._taskGroupGap;
          return extraHeight + this._marginTop;
        } else if (task.isMainTask) {
          let extraHeight = this._taskGroupGap * 2 + this._largeTaskHeight;
          return extraHeight + this._marginTop;
        } else {
          let extraHeight = this._taskGroupGap * 3 + this._largeTaskHeight * 2;
          return task.yIndex * barHeight + extraHeight + this._marginTop;
        }
      })
      .render(tasks);
  }
}

export default TaskView;
