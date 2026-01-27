import * as d3 from "d3";
import TaskYIndexAssigner from "./taskyindexassigner";
import TaskRenderer from "./taskrenderer";
import XAxisDrawer from "./xaxisdrawer";
import { Task, TaskMilestone, TaskStep } from "./task";
import { applySegmentShadingToSVG } from "./segmentshading";

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
  private _lockedMilestoneKind: string | null = null;
  private _lockedMilestoneTimeWindow: {startTime: number, endTime: number} | null = null;
  private _lockedTask: Task | null = null;

  get isTaskLocked(): boolean {
    return this._lockedTask !== null;
  }
  private _dissectionView: HTMLElement;
  private _isDissectionMode: boolean;

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
    this._isDissectionMode = false;
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

    const svg = d3.select(this._canvas)
      .select("svg")
      .attr("width", this._canvasWidth)
      .attr("height", this._canvasHeight);

    // Add click event to SVG background to clear locks
    svg.on('click', (event: MouseEvent) => {
      // Only clear if clicking directly on SVG (not on child elements)
      if (event.target === svg.node()) {
        this._clearAllLocks();
      }
    });

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

    // Update all dissection sections during zoom
    if (this._isDissectionMode) {
      this._recreateParentTaskSection();
      this._recreateCurrentTaskSection();
      this._recreateSubTasksSection();
      this._recreateDissectionMilestoneSection();
    }
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
    this._isDissectionMode = true;
    this._hideStackedMilestones();
    this._createDissectionView();
  }

  hideDissectionView() {
    this._isDissectionMode = false;
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
    // Re-render stacked milestones in normal view
    this._renderStackedMilestones();
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
      overflow-x: auto;
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

  private _createTaskSection(title: string, task: Task, bgColor: string, showSteps: boolean = true): HTMLElement {
    const section = document.createElement('div');
    section.style.cssText = `
      margin-bottom: 20px;
      overflow: visible;
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
      margin-left: -20px;
      margin-right: -20px;
      border: 1px solid #ddd;
      border-radius: 6px;
      background: #f9f9f9;
      width: ${this._canvasWidth}px;
    `;

    // Calculate task bar position and width using xScale
    const leftOffset = this._xScale(task.start_time);
    const barWidth = this._xScale(task.end_time) - this._xScale(task.start_time);

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
      const timeRange = this._endTime - this._startTime;
      const containerWidth = this._canvas.parentElement.offsetWidth - 40;
      
      // Use same grouping logic as TaskRenderer for consistency
      const milestoneGroups = this._groupMilestonesByTime(task.steps);
      
      milestoneGroups.forEach(group => {
        const stepOffset = ((group.time - this._startTime) / timeRange) * containerWidth;
        if (stepOffset >= 0 && stepOffset <= containerWidth) {
          const stepDot = document.createElement('div');
          
          // Use same styling as TaskRenderer - larger circle for multiple milestones
          const dotSize = group.steps.length > 1 ? 6 : 4;
          const hasBorder = group.steps.length > 1;
          
          stepDot.style.cssText = `
            position: absolute;
            left: ${stepOffset - dotSize/2}px;
            top: ${18 - dotSize/2}px;
            width: ${dotSize}px;
            height: ${dotSize}px;
            background: #ff0000;
            border-radius: 50%;
            border: ${hasBorder ? '1px solid #ffffff' : 'none'};
            cursor: pointer;
          `;
          
          // Add tooltip for milestone group (consistent with TaskRenderer)
          stepDot.addEventListener('mouseenter', (e) => {
            if (group.steps.length > 1) {
              // Show group info for multiple milestones
              this._showMilestoneGroupInfo(group.steps, group.time);
            } else {
              // Show single milestone info
              this._showMilestoneInfo(group.steps[0], 1);
            }
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
    taskInfo.textContent = `Type: ${task.kind}, What: ${task.what}, Location: ${task.location}, Time: ${this._smartString(task.start_time)} to ${this._smartString(task.end_time)}`;
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
      overflow: visible;
      padding-top: 35px;
      padding-bottom: 35px;
    `;

    const titleDiv = document.createElement('div');
    titleDiv.textContent = 'Milestones Achieving';
    titleDiv.style.cssText = `
      font-size: 18px;
      font-weight: bold;
      color: #333;
    `;

    // Create wrapper for milestone section
    const scrollWrapper = document.createElement('div');
    scrollWrapper.style.cssText = `
      padding-top: 35px;
      margin-left: -20px;
      margin-right: -20px;
      width: ${this._canvasWidth}px;
      overflow: visible;
    `;

    // Create timeline container - overflow visible allows labels to extend
    const milestoneContainer = document.createElement('div');
    milestoneContainer.style.cssText = `
      position: relative;
      height: 30px;
      border-top: 2px solid #666;
      border-bottom: 2px solid #666;
      background: #f0f0f0;
      overflow: visible;
    `;

    // Use same width as canvas for alignment
    const containerWidth = this._canvasWidth;
    const timeRange = this._endTime - this._startTime;

    // Create original milestone visualization
    this._createOriginalMilestoneVisualization(milestoneContainer, containerWidth, timeRange);

    scrollWrapper.appendChild(milestoneContainer);
    section.appendChild(titleDiv);
    section.appendChild(scrollWrapper);

    return section;
  }

  private _createSubTasksSection(): HTMLElement {
    const section = document.createElement('div');
    section.style.cssText = `
      margin-bottom: 20px;
      overflow: visible;
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
      // Create hierarchical subtasks display
      const totalCount = { count: 0 };
      this._createHierarchicalSubTasks(section, this._subTasks, 0, totalCount);
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

  private _createHierarchicalSubTasks(parentContainer: HTMLElement, subTasks: any[], level: number, totalCount: { count: number } = { count: 0 }) {
    if (totalCount.count >= 100 || subTasks.length === 0) {
      return;
    }

    const indentLevel = level * 20;
    const levelColors = ['#fefdca', '#f0f8e8', '#e8f2ff', '#ffeaa7', '#fab1a0', '#fd79a8', '#fdcb6e', '#6c5ce7', '#a29bfe'];
    const levelLabel = `Level ${level + 1}`;
    
    const remainingCount = 100 - totalCount.count;
    const tasksToShow = subTasks.slice(0, Math.min(subTasks.length, remainingCount));
    totalCount.count += tasksToShow.length;
    const levelContainer = document.createElement('div');
    levelContainer.style.cssText = `
      margin-left: ${indentLevel}px;
      margin-bottom: 15px;
      border-left: ${level > 0 ? '2px solid #ddd' : 'none'};
      padding-left: ${level > 0 ? '10px' : '0px'};
    `;

    const levelTitle = document.createElement('div');
    const totalShown = tasksToShow.length;
    const totalAvailable = subTasks.length;
    const titleText = totalShown < totalAvailable ? 
      `${levelLabel} (showing ${totalShown} of ${totalAvailable} tasks)` :
      `${levelLabel} (${totalShown} tasks)`;
    levelTitle.textContent = titleText;
    levelTitle.style.cssText = `
      font-size: 16px;
      font-weight: bold;
      margin-bottom: 10px;
      color: #333;
    `;

    const levelIndicator = document.createElement('span');
    levelIndicator.textContent = `[L${level + 1}]`;
    levelIndicator.style.cssText = `
      background: ${this._getLevelColor(level)};
      color: white;
      padding: 2px 6px;
      border-radius: 10px;
      font-size: 10px;
      font-weight: bold;
      margin-right: 8px;
      display: inline-block;
    `;
    levelTitle.insertBefore(levelIndicator, levelTitle.firstChild);

    levelContainer.appendChild(levelTitle);
    const timeAxisContainer = document.createElement('div');
    timeAxisContainer.style.cssText = `
      position: relative;
      height: 36px;
      margin-bottom: 15px;
      margin-left: -20px;
      margin-right: -20px;
      border: 1px solid #ddd;
      border-radius: 6px;
      background: #f9f9f9;
      width: ${this._canvasWidth}px;
      overflow: visible;
    `;

    tasksToShow.forEach((subTask, index) => {
      const leftOffset = this._xScale(subTask.start_time);
      const barWidth = this._xScale(subTask.end_time) - this._xScale(subTask.start_time);

      const taskBar = document.createElement('div');
      taskBar.style.cssText = `
        position: absolute;
        left: ${Math.max(0, leftOffset)}px;
        top: 0px;
        width: ${Math.max(10, barWidth)}px;
        height: 36px;
        background: ${levelColors[level % levelColors.length]};
        border: 1px solid #999;
        border-radius: 3px;
        cursor: pointer;
        overflow: hidden;
      `;

      const taskInfo = `${subTask.kind}: ${subTask.what} (${this._smartString(subTask.start_time)} - ${this._smartString(subTask.end_time)})`;
      taskBar.title = taskInfo;
      taskBar.addEventListener('click', () => {
        const tooltip = document.querySelector('.curr-task-info');
        if (tooltip) {
          tooltip.innerHTML = `
            <div style="text-align: left; min-width: 250px;">
              <h4>${levelLabel} Task Details</h4>
              <div style="margin-bottom: 8px;">
                <span style="background-color: #ffeb3b; padding: 2px 4px; border-radius: 3px;">Kind:</span> ${subTask.kind}<br/>
                <span style="background-color: #e3f2fd; padding: 2px 4px; border-radius: 3px;">What:</span> ${subTask.what}<br/>
                <span style="background-color: #f3e5f5; padding: 2px 4px; border-radius: 3px;">Location:</span> ${subTask.location}<br/>
                <span style="background-color: #e8f5e8; padding: 2px 4px; border-radius: 3px;">Time:</span> ${this._smartString(subTask.start_time)} - ${this._smartString(subTask.end_time)}
              </div>
            </div>
          `;
          tooltip.classList.add('showing');
        }
      });

      timeAxisContainer.appendChild(taskBar);
    });

    levelContainer.appendChild(timeAxisContainer);

    if (totalCount.count < 100 && tasksToShow.length > 0) {
      const expandButton = document.createElement('button');
      expandButton.textContent = '▼ Load Next Level Sub-tasks';
      expandButton.style.cssText = `
        background: #007bff;
        color: white;
        border: none;
        padding: 4px 8px;
        border-radius: 4px;
        font-size: 12px;
        cursor: pointer;
        margin-left: 10px;
        display: inline-block;
        vertical-align: middle;
      `;

      const childrenContainer = document.createElement('div');
      childrenContainer.style.display = 'none';
      childrenContainer.className = 'subtask-children';

      let isExpanded = false;
      let isLoaded = false;

      expandButton.addEventListener('click', async () => {
        if (!isLoaded) {
          expandButton.textContent = '⏳ Loading...';
          expandButton.disabled = true;
          
          try {
            const allChildrenPromises = tasksToShow.map(subTask => this._fetchSubTasks(subTask.id));
            const allChildrenResults = await Promise.all(allChildrenPromises);
            const allChildren = allChildrenResults.flat().filter(task => task && task.id);

            console.log(`TaskView: Got ${allChildren.length} total children for level ${level}`);
            if (allChildren.length > 0 && totalCount.count < 100) {
              this._createHierarchicalSubTasks(childrenContainer, allChildren, level + 1, totalCount);
              isLoaded = true;
              expandButton.textContent = '▲ Hide Next Level';
              expandButton.disabled = false;
              childrenContainer.style.display = 'block';
              isExpanded = true;
            } else if (totalCount.count >= 100) {
              const limitReachedDiv = document.createElement('div');
              limitReachedDiv.textContent = 'Reached limit of 100 subtasks';
              limitReachedDiv.style.cssText = `
                font-size: 12px;
                color: #ff6b6b;
                font-weight: bold;
                margin-top: 5px;
              `;
              childrenContainer.appendChild(limitReachedDiv);
              expandButton.textContent = '• Limit Reached';
              expandButton.disabled = true;
              childrenContainer.style.display = 'block';
              isLoaded = true;
            } else {
              const noChildrenDiv = document.createElement('div');
              noChildrenDiv.textContent = 'No sub-tasks found at next level';
              noChildrenDiv.style.cssText = `
                font-size: 12px;
                color: #666;
                font-style: italic;
                margin-top: 5px;
              `;
              childrenContainer.appendChild(noChildrenDiv);
              expandButton.textContent = '• No Next Level Sub-tasks';
              expandButton.disabled = true;
              childrenContainer.style.display = 'block';
              isLoaded = true;
            }
          } catch (error) {
            console.error('Error fetching subtasks:', error);
            expandButton.textContent = '❌ Error loading';
            expandButton.disabled = true;
          }
        } else {
          if (isExpanded) {
            childrenContainer.style.display = 'none';
            expandButton.textContent = '▼ Show Next Level';
            isExpanded = false;
          } else {
            childrenContainer.style.display = 'block';
            expandButton.textContent = '▲ Hide Next Level';
            isExpanded = true;
          }
        }
      });

      levelTitle.appendChild(expandButton);
      levelContainer.appendChild(childrenContainer);
    }

    parentContainer.appendChild(levelContainer);
  }

  private _getLevelColor(level: number): string {
    const colors = ['#007bff', '#28a745', '#dc3545']; // Blue, Green, Red
    return colors[level] || '#6c757d';
  }

  private async _fetchSubTasks(parentTaskId: string): Promise<any[]> {
    try {
      console.log(`TaskView: Fetching subtasks for parent ID: ${parentTaskId}`);
      
      // Include time range parameters to ensure we get relevant subtasks
      const params = new URLSearchParams();
      params.set('parentid', parentTaskId);
      params.set('starttime', this._startTime.toString());
      params.set('endtime', this._endTime.toString());
      
      const response = await fetch(`/api/trace?${params.toString()}`);
      if (!response.ok) {
        // Provide meaningful feedback about the error
        console.error(`Error fetching subtasks for ${parentTaskId}: Server responded with status ${response.status} ${response.statusText}`);
        // Optionally, you could trigger a UI update or callback here to inform the user
        return [];
      }
      const subTasks = await response.json();
      console.log(`TaskView: Received ${subTasks ? subTasks.length : 0} subtasks for parent ${parentTaskId}:`, subTasks);
      
      // Filter out any null/undefined results
      return (subTasks || []).filter(task => task && task.id);
    } catch (error) {
      // Provide meaningful feedback about the network error
      console.error(`Network error fetching subtasks for ${parentTaskId}:`, error);
      // Optionally, you could trigger a UI update or callback here to inform the user
      return [];
    }
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

  private _hideStackedMilestones() {
    const svg = d3.select(this._canvas).select("svg");
    svg.selectAll(".task-stacked-milestones").remove();
    
    // Remove legend from right column
    const existingLegend = document.querySelector('.task-milestone-kinds-legend');
    if (existingLegend) {
      existingLegend.remove();
    }
  }

  private _renderStackedMilestones() {
    // Only render in normal view mode
    if (this._isDissectionMode || !this._task) {
      return;
    }
    
    const componentName = this._task.location;
    
    // Fetch only stacked milestones data (concurrent tasks are handled by componentview)
    const stackedMilestonesParams = new URLSearchParams();
    stackedMilestonesParams.set("info_type", "ConcurrentTaskMilestones");
    stackedMilestonesParams.set("where", componentName);
    stackedMilestonesParams.set("start_time", this._startTime.toString());
    stackedMilestonesParams.set("end_time", this._endTime.toString());
    stackedMilestonesParams.set("num_dots", "40");
    
    fetch(`/api/compinfo?${stackedMilestonesParams.toString()}`)
      .then(response => response.json())
      .then((stackedData) => {
        this._renderConcurrentTasksWithStackedBars(null, stackedData);
      })
      .catch((error) => {
        console.log('TaskView error fetching data:', error);
      });
  }

  private _renderConcurrentTasksWithStackedBars(concurrentData: any, stackedData: any) {
    if (this._isDissectionMode) {
      return;
    }
    
    // Get the componentview canvas instead of taskview canvas
    const componentCanvas = document.getElementById("component-view");
    if (!componentCanvas) {
      console.log("TaskView: Component view canvas not found");
      return;
    }
    
    const svg = d3.select(componentCanvas).select("svg");
    
    svg.selectAll(".task-stacked-milestones").remove();
    
    // Only proceed if we have stacked data to render
    if (!stackedData || !stackedData.data || !stackedData.kinds) {
      console.log("TaskView: No stacked data available");
      return;
    }
    
    console.log("TaskView: Starting to render stacked bars on componentview canvas, looking for componentview elements...");
    
    // Debug: List all groups in the componentview SVG
    const allGroups = svg.selectAll("g");
    console.log("TaskView: Found", allGroups.size(), "g elements in componentview SVG");
    allGroups.each(function() {
      const group = d3.select(this);
      console.log("TaskView: Group classes:", group.attr("class"));
    });
    
    // Find the existing componentview Y scale and coordinate system
    // The concurrent line chart is already rendered by componentview, we just need to add stacked bars to it
    const existingYAxisGroup = svg.select(".y-axis-left");
    if (existingYAxisGroup.empty()) {
      console.log("No existing Y axis found from componentview, will wait and retry");
      // ComponentView might not have rendered yet, let's wait a bit and retry
      setTimeout(() => {
        this._renderConcurrentTasksWithStackedBars(null, stackedData);
      }, 100);
      return;
    }
    
    const existingCurve = svg.select(".curve-ConcurrentTask .line");
    if (existingCurve.empty()) {
      console.log("No existing concurrent task curve found from componentview");
      return;
    }
    
    const xAxisHeight = 30;
    const componentCanvasHeight = componentCanvas.offsetHeight;
    const componentMarginTop = 5;
    const chartBottom = componentCanvasHeight - xAxisHeight + 5;
    const chartTop = componentMarginTop + 2 * (componentCanvasHeight - xAxisHeight - componentMarginTop) / 3 - 15;
    const concurrentParams = new URLSearchParams();
    concurrentParams.set("info_type", "ConcurrentTask");
    concurrentParams.set("where", this._task.location);
    concurrentParams.set("start_time", this._startTime.toString());
    concurrentParams.set("end_time", this._endTime.toString());
    concurrentParams.set("num_dots", "40");
    
    fetch(`/api/compinfo?${concurrentParams.toString()}`)
      .then(response => response.json())
      .then((concurrentData) => {
        console.log('TaskView: Got concurrent data for Y scale:', concurrentData);
        console.log('TaskView: Got stacked data:', stackedData);
        
        const maxConcurrentValue = Math.max(...concurrentData.data.map((d: any) => d.value));
        console.log('TaskView: Max concurrent value:', maxConcurrentValue);
        console.log('TaskView: Chart range - bottom:', chartBottom, 'top:', chartTop);
        
        let maxStackedValue = 0;
        stackedData.data.forEach((timePoint: any) => {
          let stackTotal = 0;
          stackedData.kinds.forEach((kind: string) => {
            stackTotal += timePoint.values[kind] || 0;
          });
          maxStackedValue = Math.max(maxStackedValue, stackTotal);
        });
        console.log('TaskView: Max stacked value:', maxStackedValue);
        
        const stackedYScale = d3
          .scaleLinear()
          .domain([0, maxStackedValue])
          .range([chartBottom, chartTop]);
          
        const componentCanvasWidth = componentCanvas.offsetWidth;
        const componentMarginLeft = 5;
        const componentMarginRight = 60;
        const componentXScale = d3
          .scaleLinear()
          .domain([this._startTime, this._endTime])
          .range([componentMarginLeft, componentCanvasWidth - componentMarginRight]);
        
        console.log('TaskView: Stacked Y scale domain:', stackedYScale.domain(), 'range:', stackedYScale.range());
        console.log('TaskView: X scale domain:', componentXScale.domain(), 'range:', componentXScale.range());
        
        const stackedGroup = svg.append("g").attr("class", "task-stacked-milestones");
        
        this._drawYAxisRight(stackedGroup, stackedYScale, componentCanvasWidth);
        
        this._renderStackedBars(stackedGroup, stackedData, stackedYScale, componentXScale);
        this._addMilestoneLegendToRightColumn(stackedData.kinds);
        
        console.log('TaskView: Stacked bars rendered');
      })
      .catch((error) => {
        console.log('Error fetching concurrent data for Y scale:', error);
      });
    
  }

  private _drawYAxis(group: any, yScale: any) {
    const yAxisLeft = d3.axisLeft(yScale);
    let yAxisGroup = group.select(".concurrent-y-axis");
    if (yAxisGroup.empty()) {
      yAxisGroup = group.append("g").attr("class", "concurrent-y-axis");
    }
    
    yAxisGroup
      .attr("transform", `translate(${this._marginLeft + 35}, 0)`)
      .call(yAxisLeft.ticks(5));
    
    // Add grid lines
    const tickValues = yScale.ticks(5);
    const gridLines = yAxisGroup.selectAll(".grid-line")
      .data(tickValues);
  
    gridLines.enter()
      .append("line")
      .attr("class", "grid-line")
      .merge(gridLines as any)
      .attr("x1", 0)
      .attr("x2", this._canvasWidth - this._marginLeft - 35)
      .attr("y1", d => yScale(d))
      .attr("y2", d => yScale(d))
      .attr("stroke", "#ccc")
      .attr("stroke-dasharray", "3,3")
      .attr("opacity", 0.5);
  
    gridLines.exit().remove();
  }

  private _drawYAxisRight(group: any, yScale: any, canvasWidth: number) {
    const yAxisRight = d3.axisRight(yScale);
    let yAxisRightGroup = group.select(".stacked-y-axis-right");
    if (yAxisRightGroup.empty()) {
      yAxisRightGroup = group.append("g").attr("class", "stacked-y-axis-right");
    }
    
    yAxisRightGroup
      .attr("transform", `translate(${canvasWidth - 50}, 0)`)
      .call(yAxisRight.ticks(5).tickFormat(d3.format("d")));
    yAxisRightGroup.selectAll(".domain")
      .attr("stroke", "#666")
      .attr("stroke-width", 1);
      
    yAxisRightGroup.selectAll(".tick line")
      .attr("stroke", "#666");
      
    yAxisRightGroup.selectAll(".tick text")
      .attr("fill", "#666")
      .style("font-size", "11px");
      
    let axisLabel = yAxisRightGroup.select(".axis-label");
    if (axisLabel.empty()) {
      axisLabel = yAxisRightGroup.append("text").attr("class", "axis-label");
    }
    
    const yRange = yScale.range();
    const midY = (yRange[0] + yRange[1]) / 2;
    
    axisLabel
      .attr("transform", `translate(35, ${midY}) rotate(90)`)
      .style("text-anchor", "middle")
      .style("font-size", "12px")
      .style("fill", "#666")
      .text("Blocking Reasons");
  }


  private _renderConcurrentLine(group: any, concurrentData: any, yScale: any) {
    const pathData = concurrentData.data.map((d: any) => [d.time, d.value]);
    
    const line = d3
      .line()
      .x((d) => this._xScale(d[0]))
      .y((d) => yScale(d[1]))
      .curve(d3.curveCatmullRom.alpha(0.5));
    
    group
      .append("path")
      .datum(pathData)
      .attr("d", line)
      .attr("fill", "none")
      .attr("stroke", "#2c7bb6")
      .attr("stroke-width", 2)
      .attr("opacity", 0.8)
      .attr("class", "concurrent-line");
  }

  private _renderStackedBars(group: any, stackedData: any, yScale: any, xScale?: any) {
    console.log('TaskView: _renderStackedBars called with data:', stackedData);
    console.log('TaskView: Number of data points:', stackedData.data.length);
    console.log('TaskView: Kinds:', stackedData.kinds);
    
    // Use consistent color mapping based on sorted kinds
    const colorScale = this._createKindColorScale(stackedData.kinds);
    
    // Use the provided xScale if available, otherwise fall back to taskview's xScale
    const effectiveXScale = xScale || this._xScale;
    const effectiveCanvasWidth = xScale ? xScale.range()[1] - xScale.range()[0] : this._canvasWidth;
    
    const barWidth = Math.max(3, Math.min(8, effectiveCanvasWidth / stackedData.data.length * 0.6));
    
    console.log('TaskView: Bar width:', barWidth, 'using xScale range:', effectiveXScale.range());
    
    // Calculate time range for each data point (for tooltip)
    const timeRange = this._endTime - this._startTime;
    const numDots = stackedData.data.length;
    const binDuration = timeRange / numDots;
    
    stackedData.data.forEach((timePoint: any, index: number) => {
      const x = effectiveXScale(timePoint.time) - barWidth / 2;
      let stackBase = yScale(0); // Start from bottom
      
      // Calculate time range for this data point
      const binStartTime = this._startTime + index * binDuration;
      const binEndTime = this._startTime + (index + 1) * binDuration;
      
      stackedData.kinds.forEach((kind: string) => {
        const value = timePoint.values[kind] || 0;
        if (value > 0) {
          const segmentHeight = yScale(0) - yScale(value);
          
          const rect = group
            .append("rect")
            .attr("x", x)
            .attr("y", stackBase - segmentHeight)
            .attr("width", barWidth)
            .attr("height", segmentHeight)
            .attr("fill", colorScale(kind))
            .attr("opacity", 0.8)
            .attr("stroke", "#fff")
            .attr("stroke-width", 0.5)
            .attr("class", "stacked-milestone-bar")
            .style("cursor", "pointer");
          
          // Add hover and click events for tooltip
          rect
            .on("mouseenter", async (event) => {
              // Only show hover effect if not locked or locked to different item
              if (!this._lockedMilestoneKind || 
                  (this._lockedMilestoneKind !== kind || 
                   !this._lockedMilestoneTimeWindow ||
                   this._lockedMilestoneTimeWindow.startTime !== binStartTime ||
                   this._lockedMilestoneTimeWindow.endTime !== binEndTime)) {
                console.log(`TaskView: Mouse entered stacked bar - kind: ${kind}, value: ${value}, timeWindow: ${binStartTime}-${binEndTime}`);
                this._showStackedBarTooltip(kind, value, binStartTime, binEndTime, timePoint.time);
                // Highlight the segment
                rect.attr("opacity", 0.6);
                // Highlight corresponding tasks that have milestones in this time window
                try {
                  await this._highlightTasksWithMilestonesInTimeWindow(binStartTime, binEndTime, kind);
                } catch (error) {
                  console.error('TaskView: Error in highlighting:', error);
                }
              }
            })
            .on("mouseleave", () => {
              // Only hide if not locked
              if (!this._lockedMilestoneKind) {
                this._hideStackedBarTooltip();
                // Restore normal opacity
                rect.attr("opacity", 0.8);
                // Clear task highlighting
                this._clearTaskHighlighting();
              }
            })
            .on("click", async (event) => {
              event.stopPropagation();
              console.log(`TaskView: Clicked stacked bar - kind: ${kind}, value: ${value}, timeWindow: ${binStartTime}-${binEndTime}`);
              
              // Check if clicking the same item that's already locked
              if (this._lockedMilestoneKind === kind && 
                  this._lockedMilestoneTimeWindow &&
                  this._lockedMilestoneTimeWindow.startTime === binStartTime &&
                  this._lockedMilestoneTimeWindow.endTime === binEndTime) {
                // Unlock
                this._lockedMilestoneKind = null;
                this._lockedMilestoneTimeWindow = null;
                this._hideStackedBarTooltip();
                rect.attr("opacity", 0.8);
                this._clearTaskHighlighting();
                console.log('TaskView: Unlocked milestone highlighting');
              } else {
                // Lock to this item
                this._lockedMilestoneKind = kind;
                this._lockedMilestoneTimeWindow = {startTime: binStartTime, endTime: binEndTime};
                
                // Clear any existing highlighting first
                this._clearTaskHighlighting();
                d3.selectAll('.stacked-bar rect').attr("opacity", 0.8);
                
                // Show locked state
                this._showStackedBarTooltip(kind, value, binStartTime, binEndTime, timePoint.time);
                rect.attr("opacity", 0.6);
                try {
                  await this._highlightTasksWithMilestonesInTimeWindow(binStartTime, binEndTime, kind);
                } catch (error) {
                  console.error('TaskView: Error in highlighting:', error);
                }
                console.log(`TaskView: Locked milestone highlighting for ${kind}`);
              }
            });
          
          stackBase -= segmentHeight;
        }
      });
    });
  }

  handleTaskClick(task: Task) {
    console.log(`TaskView: Clicked task ${task.id}`);
    
    // Check if clicking the same task that's already locked
    if (this._lockedTask && this._lockedTask.id === task.id) {
      // Unlock
      this._lockedTask = null;
      this._hideTaskTooltip();
      console.log('TaskView: Unlocked task tooltip');
    } else {
      // Lock to this task
      this._lockedTask = task;
      this._showTaskTooltip(task);
      console.log(`TaskView: Locked task tooltip for ${task.id}`);
    }
  }

  private _showTaskTooltip(task: Task) {
    console.log(`TaskView: Showing task tooltip for ${task.id}`);
    const tooltip = document.querySelector('.curr-task-info');
    if (tooltip) {
      const tableLeftCol = 3;
      const tableRightcol = 12 - tableLeftCol;

      tooltip.innerHTML = `
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
        <dd class="col-sm-${tableRightcol}">${task.location}</dd>

        <dt class="col-sm-${tableLeftCol}">Start</dt>
        <dd class="col-sm-${tableRightcol}">
            ${this._smartString(task['start_time'])}
        </dd>

        <dt class="col-sm-${tableLeftCol}">End</dt>
        <dd class="col-sm-${tableRightcol}">
            ${this._smartString(task['end_time'])}
        </dd>

        <dt class="col-sm-${tableLeftCol}">Duration</dt>
        <dd class="col-sm-${tableRightcol}">
            ${this._smartString(task['end_time'] - task['start_time'])}
        </dd>
    </dl>
</div>`;
      tooltip.classList.add('showing');
      // Also highlight the task when showing tooltip
      this.highlight(task);
      console.log('TaskView: Task tooltip set and showing class added');
    } else {
      console.error('TaskView: Could not find .curr-task-info element');
    }
  }

  private _hideTaskTooltip() {
    const tooltip = document.querySelector('.curr-task-info');
    if (tooltip) {
      tooltip.classList.remove('showing');
    }
    // Also clear task highlighting when hiding tooltip
    this.highlight(null);
  }

  private _clearAllLocks() {
    // Clear milestone lock
    if (this._lockedMilestoneKind) {
      this._lockedMilestoneKind = null;
      this._lockedMilestoneTimeWindow = null;
      this._hideStackedBarTooltip();
      d3.selectAll('.stacked-milestone-bar').attr("opacity", 0.8);
      this._clearTaskHighlighting();
    }
    
    // Clear task lock
    if (this._lockedTask) {
      this._lockedTask = null;
      this._hideTaskTooltip();
    }
    
    console.log('TaskView: Cleared all locks');
  }

  private _showStackedBarTooltip(kind: string, count: number, startTime: number, endTime: number, centerTime: number) {
    // Show milestone info in right column tooltip
    const tooltip = document.querySelector('.curr-task-info');
    if (tooltip) {
      tooltip.innerHTML = `
        <div style="text-align: left; min-width: 250px;">
          <h4>Blocking Reasons Analysis</h4>
          <div style="margin-bottom: 8px;">
            <span style="background-color: #ffeb3b; padding: 2px 4px; border-radius: 3px;">Blocking Reason:</span> ${kind}<br/>
            <span style="background-color: #e3f2fd; padding: 2px 4px; border-radius: 3px;">Count:</span> ${count} concurrent task${count > 1 ? 's' : ''}<br/>
            <span style="background-color: #f3e5f5; padding: 2px 4px; border-radius: 3px;">Time Point:</span> ${this._smartString(centerTime)}
          </div>
          <div style="font-size: 12px; color: #666; margin-top: 8px;">
            This shows ${count} concurrent tasks that are blocked by "${kind}" at this time point.
          </div>
        </div>
      `;
      tooltip.classList.add('showing');
    }
  }

  private _hideStackedBarTooltip() {
    const tooltip = document.querySelector('.curr-task-info');
    if (tooltip) {
      tooltip.classList.remove('showing');
    }
  }

  private _getParentTaskColor(): string {
    // Get parent task color from the task renderer's color coder
    if (this._parentTask && this._taskRenderer && this._taskRenderer._colorCoder) {
      try {
        return this._taskRenderer._colorCoder.lookup(this._parentTask);
      } catch (e) {
        console.log('TaskView: Could not get parent task color:', e);
      }
    }
    
    // Fallback to a light blue color similar to the original
    return '#a5dee5';
  }

  private _createKindColorScale(kinds: string[]) {
    // Define consistent colors for milestone kinds
    const colors = ['#ff7f0e', '#2ca02c', '#d62728', '#9467bd', '#8c564b', '#e377c2', '#7f7f7f', '#bcbd22', '#17becf'];
    
    // Sort kinds to ensure consistent ordering
    const sortedKinds = [...kinds].sort();
    
    // Create a mapping from kind to color based on sorted order
    const kindToColorMap = {};
    sortedKinds.forEach((kind, index) => {
      kindToColorMap[kind] = colors[index % colors.length];
    });
    
    // Create d3 color scale that uses the mapping
    return (kind: string) => kindToColorMap[kind] || '#666666'; // fallback color
  }

  private _addMilestoneLegendToRightColumn(kinds: string[]) {
    if (this._isDissectionMode) {
      return;
    }
    
    // Remove existing milestone legend
    const existingLegend = document.querySelector('.task-milestone-kinds-legend');
    if (existingLegend) {
      existingLegend.remove();
    }
    
    // Find the right column
    const tooltip = document.querySelector('.curr-task-info');
    if (!tooltip || !tooltip.parentElement) {
      console.log('Could not find right column to add TaskView legend');
      return;
    }
    
    const rightColumn = tooltip.parentElement;
    const colorScale = this._createKindColorScale(kinds);
    
    // Create legend container
    const legendContainer = document.createElement('div');
    legendContainer.className = 'task-milestone-kinds-legend';
    legendContainer.style.cssText = `
      margin-top: 20px;
      padding: 15px;
      background: #f8f9fa;
      border: 1px solid #dee2e6;
      border-radius: 6px;
      font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
    `;
    
    // Add title
    const title = document.createElement('div');
    title.textContent = 'Blocking Reasons Flow';
    title.style.cssText = `
      font-size: 14px;
      font-weight: bold;
      margin-bottom: 15px;
      color: #343a40;
      border-bottom: 1px solid #dee2e6;
      padding-bottom: 5px;
    `;
    legendContainer.appendChild(title);
    
    // Define component milestone flows based on our analysis
    const componentFlows = this._getComponentMilestoneFlows(kinds);
    
    // Render each component flow
    componentFlows.forEach((componentFlow, componentIndex) => {
      if (componentFlow.kinds.length === 0) return;
      
      // Component title
      const componentTitle = document.createElement('div');
      componentTitle.textContent = componentFlow.component;
      componentTitle.style.cssText = `
        font-size: 13px;
        font-weight: bold;
        margin-bottom: 8px;
        margin-top: ${componentIndex > 0 ? '15px' : '0px'};
        color: #495057;
        background: #e9ecef;
        padding: 4px 8px;
        border-radius: 4px;
      `;
      legendContainer.appendChild(componentTitle);
      
      // Render kinds vertically with arrows between them
      componentFlow.kinds.forEach((kind, index) => {
        // Item container for each milestone kind
        const item = document.createElement('div');
        item.style.cssText = `
          display: flex;
          align-items: center;
          margin-bottom: 6px;
          font-size: 12px;
        `;
        
        // Color square
        const colorBox = document.createElement('div');
        colorBox.style.cssText = `
          width: 12px;
          height: 12px;
          background-color: ${colorScale(kind)};
          border: 1px solid #dee2e6;
          margin-right: 8px;
          border-radius: 2px;
        `;
        
        // Text label
        const label = document.createElement('span');
        label.textContent = kind;
        label.style.cssText = `
          color: #495057;
          font-weight: 500;
        `;
        
        item.appendChild(colorBox);
        item.appendChild(label);
        legendContainer.appendChild(item);
        
        // Add arrow if not the last item
        if (index < componentFlow.kinds.length - 1) {
          const arrow = document.createElement('div');
          arrow.textContent = '↓';
          arrow.style.cssText = `
            text-align: center;
            margin: 2px 0;
            color: #6c757d;
            font-weight: bold;
            font-size: 14px;
          `;
          legendContainer.appendChild(arrow);
        }
      });
    });
    
    // Add to right column at the bottom
    rightColumn.appendChild(legendContainer);
  }
  
  private _getComponentMilestoneFlows(kinds: string[]): Array<{component: string, kinds: string[]}> {
    // Define milestone flows for each component based on our analysis
    // AddressTranslator: network_busy → translation → data → subtask → network_busy
    const addressTranslatorFlow = ['network_busy', 'translation', 'data', 'subtask', 'network_busy'];
    // ROB: network_transfer → queue → hardware_resource → network_busy → data → dependency → network_busy
    const robFlow = ['network_transfer', 'queue', 'hardware_resource', 'network_busy', 'data', 'dependency', 'network_busy'];
    
    const componentFlows = [];
    
    // Get current task's component name to determine which flow to show
    const currentComponentName = this._task?.location || '';
    
    // Check if current component is AddressTranslator
    if (currentComponentName.toLowerCase().includes('addresstranslator')) {
      // Always use _sortKindsByFlow to ensure correct order
      const sortedKinds = this._sortKindsByFlow(kinds, addressTranslatorFlow);
      componentFlows.push({
        component: 'AddressTranslator',
        kinds: sortedKinds
      });
    }
    // Check if current component is ROB
    else if (currentComponentName.toLowerCase().includes('rob') || currentComponentName.toLowerCase().includes('reorder')) {
      // Always use _sortKindsByFlow to ensure correct order
      const sortedKinds = this._sortKindsByFlow(kinds, robFlow);
      componentFlows.push({
        component: 'ROB (Reorder Buffer)',
        kinds: sortedKinds
      });
    }
    // For other components, try to match patterns or show all kinds
    else {
      // Check which component patterns have more matches
      const addressTranslatorKinds = addressTranslatorFlow.filter(kind => kinds.includes(kind));
      const robKinds = robFlow.filter(kind => kinds.includes(kind));
      
      if (addressTranslatorKinds.length > robKinds.length && addressTranslatorKinds.length > 0) {
        componentFlows.push({
          component: 'AddressTranslator Pattern',
          kinds: addressTranslatorKinds
        });
      } else if (robKinds.length > 0) {
        componentFlows.push({
          component: 'ROB Pattern',
          kinds: robKinds
        });
      } else {
        // Show all kinds in alphabetical order as fallback
        componentFlows.push({
          component: currentComponentName || 'Component',
          kinds: [...kinds].sort()
        });
      }
    }
    
    return componentFlows;
  }
  
  private _sortKindsByFlow(kinds: string[], preferredFlow: string[]): string[] {
    // Sort kinds according to the preferred flow order
    const sortedKinds: string[] = [];
    
    // Add kinds in flow order if they exist, but only unique ones
    const addedKinds = new Set<string>();
    preferredFlow.forEach(flowKind => {
      // Find matching kind (case insensitive)
      const matchingKind = kinds.find(k => k.toLowerCase() === flowKind.toLowerCase());
      if (matchingKind && !addedKinds.has(matchingKind)) {
        sortedKinds.push(matchingKind);
        addedKinds.add(matchingKind);
      }
    });
    
    // Add any remaining kinds alphabetically
    kinds.forEach(kind => {
      if (!addedKinds.has(kind)) {
        sortedKinds.push(kind);
        addedKinds.add(kind);
      }
    });
    
    return sortedKinds;
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

  private async _highlightTasksWithMilestonesInTimeWindow(startTime: number, endTime: number, kind: string) {
    console.log(`🎯 TaskView: _highlightTasksWithMilestonesInTimeWindow called with startTime=${startTime}, endTime=${endTime}, kind=${kind}`);

    // Clear any existing highlighting first
    this._clearTaskHighlighting();

    // Calculate bin midpoint to match backend logic (componentinfo.go:928)
    const instantTime = startTime + (endTime - startTime) / 2;

    // Use the SAME logic as the stacked bar calculation in Go code:
    // 1. Find tasks that are running in this time window (task time overlaps with [startTime, endTime])
    // 2. For each running task, find their current milestone kind at instantTime (bin midpoint)
    // 3. Highlight only those tasks whose current milestone kind matches the hovered kind

    try {
      const componentName = this._task?.location;
      if (!componentName) {
        console.log('TaskView: No component name available for highlighting');
        return;
      }

      const params = new URLSearchParams();
      params.set('where', componentName);
      params.set('starttime', startTime.toString());
      params.set('endtime', endTime.toString());

      const response = await fetch(`/api/trace?${params.toString()}`);
      if (!response.ok) {
        console.error('TaskView: Failed to fetch tasks for highlighting');
        return;
      }

      const allComponentTasks = await response.json();
      console.log(`TaskView: Found ${allComponentTasks.length} tasks in component ${componentName}`);
      
      const tasksToHighlight: any[] = [];
      
      allComponentTasks.forEach((task: any) => {
        // 1. Check if task is running in this time window (same logic as Go isTaskRunningInBin)
        const isRunning = !(task.end_time < startTime || task.start_time > endTime);
        
        if (!isRunning) {
          return;
        }
        
        // 2. Find task's current blocking reasons (same logic as Go findTaskCurrentBlockingReasons)
        let currentBlockingReasons: string[] = [];

        if (task.steps && task.steps.length > 0) {
          // Find all future milestones after instantTime (bin midpoint)
          const futureSteps: any[] = [];
          task.steps.forEach((step: any) => {
            if (step.time > instantTime) {
              futureSteps.push(step);
            }
          });
          
          if (futureSteps.length > 0) {
            // Find the earliest future milestone time point
            let minTime = futureSteps[0].time;
            futureSteps.forEach((step: any) => {
              if (step.time < minTime) {
                minTime = step.time;
              }
            });
            
            // Collect all milestone kinds at this earliest time point
            futureSteps.forEach((step: any) => {
              if (step.time === minTime) {
                currentBlockingReasons.push(step.kind);
              }
            });
          }
        }
        
        // 3. Check if any current blocking reason matches the hovered kind
        if (currentBlockingReasons.includes(kind)) {
          tasksToHighlight.push(task);
          console.log(`TaskView: Task ${task.id} is running and has blocking reasons: ${currentBlockingReasons.join(', ')}`);
        }
      });

    // Highlight the matching tasks by modifying their visual appearance in COMPONENT view
    const componentCanvas = document.getElementById("component-view");
    if (!componentCanvas) {
      console.log('TaskView: Component view canvas not found for highlighting');
      return;
    }
    
    const componentSvg = d3.select(componentCanvas).select('svg');
    const taskBarGroup = componentSvg.select('.task-bar');
    
    console.log('TaskView: Task bar group found:', !taskBarGroup.empty());
    console.log('TaskView: Total rects in task bar group:', taskBarGroup.selectAll('rect').size());
    
    // Debug: List all rendered task IDs
    const renderedTaskIds: string[] = [];
    taskBarGroup.selectAll('rect').each(function(d: any) {
      if (d && d.id) {
        renderedTaskIds.push(d.id);
      }
    });
    console.log('TaskView: Rendered task IDs (first 10):', renderedTaskIds.slice(0, 10));
    
    // Debug: Check which highlighted tasks are actually rendered
    const highlightableTaskIds = tasksToHighlight.map(t => t.id);
    const intersection = renderedTaskIds.filter(id => highlightableTaskIds.includes(id));
    console.log(`TaskView: ${intersection.length} of ${highlightableTaskIds.length} highlighted tasks are actually rendered`);
    console.log('TaskView: Intersecting task IDs:', intersection);
    
    if (!taskBarGroup.empty()) {
      // First, dim all task bars (like TaskRenderer.hightlight does)
      taskBarGroup
        .selectAll('rect')
        .attr('opacity', 0.4)
        .attr('stroke-opacity', 0.2);
      
      // Then highlight the matching tasks that are actually rendered in taskview
      let highlightedCount = 0;
      tasksToHighlight.forEach(task => {
        // Convert task ID to the format used by TaskRenderer
        const taskIdTag = this._taskIdTag(task.id);
        const taskRect = taskBarGroup.select(`#task-${taskIdTag}`);
        
        if (!taskRect.empty()) {
          taskRect
            .attr('opacity', 1.0)
            .attr('stroke-opacity', 0.8)
            .classed('highlighted-task', true);
          highlightedCount++;
          console.log(`TaskView: Successfully highlighted task ${taskIdTag}`);
        }
      });
      
      console.log(`TaskView: Highlighted ${highlightedCount} tasks out of ${tasksToHighlight.length} matching tasks`);
    } else {
      console.log('TaskView: Task bar group not found in SVG');
    }

    console.log(`TaskView: Found ${tasksToHighlight.length} tasks with ${kind} milestones in time window ${startTime}-${endTime}`);
    
    } catch (error) {
      console.error('TaskView: Error in highlighting tasks:', error);
    }
  }

  private _taskIdTag(taskId: string): string {
    // Same logic as TaskRenderer._taskIdTag method
    return taskId
      .replace(/@/g, "-")
      .replace(/\./g, '-')
      .replace(/\[/g, '-')
      .replace(/\]/g, '-')
      .replace(/_/g, '-');
  }

  private _clearTaskHighlighting() {
    // Remove highlighting from all tasks in COMPONENT view (restore to normal opacity like TaskRenderer does)
    const componentCanvas = document.getElementById("component-view");
    if (!componentCanvas) {
      return;
    }
    
    const componentSvg = d3.select(componentCanvas).select('svg');
    const taskBarGroup = componentSvg.select('.task-bar');
    
    if (!taskBarGroup.empty()) {
      // Restore all task rectangles to normal opacity (like TaskRenderer.hightlight(null))
      taskBarGroup.selectAll('rect')
        .attr('opacity', 1.0)
        .attr('stroke-opacity', 0.2)
        .classed('highlighted-task', false);
    }
  }

  private _createStackedMilestoneVisualization(container: HTMLElement, containerWidth: number, timeRange: number) {
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
    const milestonesByKind: { [key: string]: any[] } = {};
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
      const kindCounts: { [key: string]: number } = {};
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

  private _showStackedTooltip(event: MouseEvent, kind: string, count: number, startTime: number, endTime: number) {
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

  private _hideStackedTooltip() {
    const tooltip = document.querySelector('.curr-task-info');
    if (tooltip) {
      tooltip.classList.remove('showing');
    }
  }

  private _createOriginalMilestoneVisualization(container: HTMLElement, containerWidth: number, timeRange: number) {
    // Use task steps as milestone points (these are the red dots in the upper view)
    if (this._task && this._task.steps && this._task.steps.length > 0) {
      const sortedSteps = [...this._task.steps].sort((a, b) => a.time - b.time);
      // Use rainbow color scheme for milestone achieving (no duplicate colors)
      const milestoneColors = ['#FF6B6B', '#FFD93D', '#52C41A', '#9B59B6', '#FF8C00', '#1E90FF', '#20B2AA'];
      
      // Group milestones by time point, same logic as red dots
      const milestoneGroups = this._groupMilestonesByTime(sortedSteps);

      // Milestone bar should start from the first milestone, not before it
      const firstGroupTime = milestoneGroups[0].time;
      const lastGroupTime = milestoneGroups[milestoneGroups.length - 1].time;

      // Calculate milestone bar start position and total width using xScale
      // Background bar should end before the last milestone divider line
      const milestoneBarStartX = this._xScale(firstGroupTime);
      const milestoneBarEndX = this._xScale(lastGroupTime);
      const milestoneBarWidth = milestoneBarEndX - milestoneBarStartX - 2; // -2 to end before last divider line (2px width)
      
      // Create milestone bar background with parent task color
      const parentTaskColor = this._getParentTaskColor();
      const milestoneBackground = document.createElement('div');
      milestoneBackground.style.cssText = `
        position: absolute;
        left: ${Math.max(0, milestoneBarStartX)}px;
        top: 0;
        width: ${Math.max(10, milestoneBarWidth)}px;
        height: 100%;
        background: ${parentTaskColor};
        z-index: 1;
        pointer-events: none;
      `;
      container.appendChild(milestoneBackground);
      
      // Create color blocks between milestones
      // Structure: milestone[0] -> colorBlock[0] (shows milestone[1]) -> milestone[1] -> ... -> milestone[n-1] (no block after last)

      // Create color blocks from first milestone to second-to-last milestone
      // Each block shows the info of the milestone at its END
      for (let i = 0; i < milestoneGroups.length - 1; i++) {
        const blockStartTime = milestoneGroups[i].time;
        const blockEndTime = milestoneGroups[i + 1].time;
        const blockStartX = this._xScale(blockStartTime);
        const blockEndX = this._xScale(blockEndTime);
        const blockWidth = blockEndX - blockStartX;

        if (blockWidth > 0) {
          const colorBlock = document.createElement('div');
          const gradientColor = milestoneColors[i % milestoneColors.length];
          const darkerColor = this._getDarkerColor(gradientColor);

          colorBlock.style.cssText = `
            position: absolute;
            left: ${blockStartX}px;
            top: 0;
            width: ${blockWidth}px;
            height: 100%;
            background: linear-gradient(to right, ${gradientColor}40, ${darkerColor});
            cursor: pointer;
            transition: opacity 0.2s ease;
            z-index: 5;
          `;

          // Click shows the milestone at the END of this block (i+1)
          const targetMilestone = milestoneGroups[i + 1];
          colorBlock.addEventListener('click', () => {
            this._showMilestoneGroupInfo(targetMilestone.steps, targetMilestone.time);
          });

          colorBlock.addEventListener('mouseenter', () => {
            colorBlock.style.opacity = '0.8';
          });

          colorBlock.addEventListener('mouseleave', () => {
            colorBlock.style.opacity = '1';
          });

          container.appendChild(colorBlock);
        }
      }
      
      // Add milestone divider lines based on groups
      milestoneGroups.forEach((group, index) => {
        const milestoneX = this._xScale(group.time);
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

        // Add label for ALL milestones (1-indexed)
        this._createMilestoneLabel(container, index + 1, milestoneX);

        // Add red flag on the last milestone divider line
        if (index === milestoneGroups.length - 1) {
          const flag = document.createElement('div');
          flag.innerHTML = '🚩';
          flag.style.cssText = `
            position: absolute;
            left: ${milestoneX + 2}px;
            top: -22px;
            font-size: 20px;
            z-index: 15;
            line-height: 1;
            pointer-events: none;
          `;
          container.appendChild(flag);
        }
      });
    }
  }

  private _createStackedLegend(container: HTMLElement, kinds: string[], colors: string[]) {
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

  private _createMilestoneLabel(
    container: HTMLElement,
    index: number,
    xPixels: number
  ): HTMLElement {
    const labelContainer = document.createElement('div');
    labelContainer.className = 'milestone-label';
    labelContainer.style.cssText = `
      position: absolute;
      margin-top: 20px;
      left: ${xPixels - 5}px;
      top: 2px;
      width: 80px;
      height: 
      text-align: center;
      font-size: 12px;
      font-weight: bold;
      color: #333;
      pointer-events: none;
      z-index: 20;
    `;

    const arrow = document.createElement('div');
    arrow.textContent = '↑';
    arrow.style.cssText = `
      font-size: 16px;
      line-height: 1;
      margin-bottom: 2px;
    `;

    const text = document.createElement('div');
    text.textContent = `Milestone ${index}`;
    text.style.cssText = `
      font-size: 11px;
      white-space: nowrap;
    `;

    labelContainer.appendChild(arrow);
    labelContainer.appendChild(text);
    container.appendChild(labelContainer);

    return labelContainer;
  }

  private _recreateDissectionMilestoneSection() {
    if (!this._dissectionView || !this._isDissectionMode) {
      return;
    }

    // Find milestone section
    const sections = this._dissectionView.children;
    let milestoneSection: HTMLElement = null;

    for (let i = 0; i < sections.length; i++) {
      const section = sections[i] as HTMLElement;
      const title = section.querySelector('div');
      if (title && title.textContent === 'Milestones Achieving') {
        milestoneSection = section;
        break;
      }
    }

    if (!milestoneSection) {
      return;
    }

    // Ensure section allows overflow for labels/flag
    milestoneSection.style.overflow = 'visible';
    milestoneSection.style.paddingTop = '35px';

    // Remove all children except title
    const children = Array.from(milestoneSection.children);
    children.forEach((child, index) => {
      if (index > 0) { // Keep first child (title)
        child.remove();
      }
    });

    // Recreate with updated xScale positions
    const containerWidth = this._canvasWidth;
    const timeRange = this._endTime - this._startTime;

    // Create wrapper for milestone section
    const scrollWrapper = document.createElement('div');
    scrollWrapper.style.cssText = `
      padding-top: 35px;
      padding-bottom: 35px;
      margin-left: -20px;
      margin-right: -20px;
      width: ${this._canvasWidth}px;
      overflow: visible;
    `;

    // Create timeline container - overflow visible allows labels to extend
    const milestoneContainer = document.createElement('div');
    milestoneContainer.style.cssText = `
      position: relative;
      height: 30px;
      border-top: 2px solid #666;
      border-bottom: 2px solid #666;
      background: #f0f0f0;
      overflow: visible;
    `;

    this._createOriginalMilestoneVisualization(milestoneContainer, containerWidth, timeRange);
    scrollWrapper.appendChild(milestoneContainer);
    milestoneSection.appendChild(scrollWrapper);
  }

  private _recreateParentTaskSection() {
    if (!this._dissectionView || !this._isDissectionMode || !this._parentTask) {
      return;
    }

    // Find and remove old Parent Task section
    const sections = this._dissectionView.children;
    for (let i = 0; i < sections.length; i++) {
      const section = sections[i] as HTMLElement;
      const title = section.querySelector('div');
      if (title && title.textContent === 'Parent Task') {
        section.remove();
        break;
      }
    }

    // Recreate with updated xScale positions
    const parentSection = this._createTaskSection('Parent Task', this._parentTask, '#a5dee5', false);

    // Insert at beginning (Parent Task is always first)
    const firstChild = this._dissectionView.children[0];
    this._dissectionView.insertBefore(parentSection, firstChild);
  }

  private _recreateCurrentTaskSection() {
    if (!this._dissectionView || !this._isDissectionMode) {
      return;
    }

    // Find and remove old Current Task section
    const sections = this._dissectionView.children;
    for (let i = 0; i < sections.length; i++) {
      const section = sections[i] as HTMLElement;
      const title = section.querySelector('div');
      if (title && title.textContent === 'Current Task') {
        section.remove();
        break;
      }
    }

    // Recreate with updated xScale positions
    const currentSection = this._createTaskSection('Current Task', this._task, '#e0f9b5', false);

    // Insert after Parent Task (if exists) or at beginning
    let insertPosition = 0;
    for (let i = 0; i < this._dissectionView.children.length; i++) {
      const section = this._dissectionView.children[i] as HTMLElement;
      const title = section.querySelector('div');
      if (title && title.textContent === 'Parent Task') {
        insertPosition = i + 1;
        break;
      }
    }

    if (insertPosition < this._dissectionView.children.length) {
      this._dissectionView.insertBefore(currentSection, this._dissectionView.children[insertPosition]);
    } else {
      this._dissectionView.appendChild(currentSection);
    }
  }

  private _recreateSubTasksSection() {
    if (!this._dissectionView || !this._isDissectionMode) {
      return;
    }

    // Find and remove old Sub Tasks section
    const sections = this._dissectionView.children;
    for (let i = 0; i < sections.length; i++) {
      const section = sections[i] as HTMLElement;
      const title = section.querySelector('div');
      if (title && title.textContent === 'Sub Tasks') {
        section.remove();
        break;
      }
    }

    // Recreate with updated xScale positions
    const subTasksSection = this._createSubTasksSection();

    // Insert before Milestones Achieving section (or at end if not found)
    let insertBeforeMilestones: Element | null = null;
    for (let i = 0; i < this._dissectionView.children.length; i++) {
      const section = this._dissectionView.children[i] as HTMLElement;
      const title = section.querySelector('div');
      if (title && title.textContent === 'Milestones Achieving') {
        insertBeforeMilestones = this._dissectionView.children[i];
        break;
      }
    }

    this._dissectionView.insertBefore(subTasksSection, insertBeforeMilestones);
  }

  setTimeAxis(startTime: number, endTime: number) {
    this._startTime = startTime;
    this._endTime = endTime;
    this._xAxisDrawer.setTimeRange(startTime, endTime);
    this._updateTimeScale();

    // Update all dissection sections during zoom
    if (this._isDissectionMode) {
      this._recreateParentTaskSection();
      this._recreateCurrentTaskSection();
      this._recreateSubTasksSection();
      this._recreateDissectionMilestoneSection();
    }
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
    
    // Render stacked milestone bars in normal view
    this._renderStackedMilestones();
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

    // Apply segment shading for non-traced periods
    this._applySegmentShading();
  }

  private _applySegmentShading() {
    if (!this._canvas || !this._xScale || this._startTime >= this._endTime) {
      return;
    }

    const svg = d3.select(this._canvas).select<SVGSVGElement>("svg");
    if (svg.empty()) {
      return;
    }

    applySegmentShadingToSVG(
      svg as any,
      this._xScale,
      this._startTime,
      this._endTime,
      this._canvasHeight - this._marginBottom,
      this._marginTop,
      "taskview-segment-shading"
    );
  }
}

export default TaskView;
