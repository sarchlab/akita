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

    // Remove existing legend
    const existingLegend = this._canvas.querySelector('.task-color-legend');
    if (existingLegend) {
      existingLegend.remove();
    }

    // Hide the SVG instead of clearing it
    const svg = this._canvas.querySelector('svg');
    if (svg) {
      svg.style.display = 'none';
    }

    if (colors.length === 0) {
      return;
    }

    // Create legend container with island style (same margin/padding as milestone legend)
    const legendContainer = document.createElement('div');
    legendContainer.className = 'task-color-legend';
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
    title.textContent = 'Task Types';
    title.style.cssText = `
      font-size: 14px;
      font-weight: bold;
      margin-bottom: 15px;
      color: #343a40;
      border-bottom: 1px solid #dee2e6;
      padding-bottom: 5px;
    `;
    legendContainer.appendChild(title);

    // Add each task type
    colors.forEach(([kindWhat, color]) => {
      // Item container
      const item = document.createElement('div');
      item.style.cssText = `
        display: flex;
        align-items: center;
        margin-bottom: 8px;
        font-size: 12px;
        cursor: pointer;
        padding: 4px;
        border-radius: 3px;
        transition: background-color 0.2s ease;
      `;
      
      // Add hover effects
      item.addEventListener('mouseenter', () => {
        item.style.backgroundColor = '#e9ecef';
        this._taskPage.highlight((t: Task) => {
          const taskKindWhat = `${t.kind}-${t.what}`;
          return taskKindWhat === kindWhat;
        });
      });
      
      item.addEventListener('mouseleave', () => {
        item.style.backgroundColor = 'transparent';
        this._taskPage.highlight(null);
      });

      // Color square
      const colorBox = document.createElement('div');
      colorBox.style.cssText = `
        width: 14px;
        height: 14px;
        background-color: ${this._colorCoder.lookupWithText(kindWhat)};
        border: 1px solid #dee2e6;
        margin-right: 10px;
        border-radius: 2px;
        flex-shrink: 0;
      `;

      // Text label
      const label = document.createElement('span');
      label.textContent = kindWhat;
      label.style.cssText = `
        color: #495057;
        font-weight: 500;
        line-height: 1.2;
      `;

      item.appendChild(colorBox);
      item.appendChild(label);
      legendContainer.appendChild(item);
    });

    // Append to canvas
    this._canvas.appendChild(legendContainer);
  }
}

export default Legend;