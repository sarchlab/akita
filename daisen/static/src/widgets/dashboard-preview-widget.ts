import { WidgetBase, WidgetConfig } from './widget-base';
import Widget from '../widget';

export class DashboardPreviewWidget extends WidgetBase {
  private _previewWidgets: Widget[] = [];
  private _startTime: number = 0;
  private _endTime: number = 0;
  private _componentNames: string[] = [];

  constructor(container: HTMLDivElement, config: WidgetConfig) {
    super(container, config);
  }

  async getData(): Promise<any> {
    const [simulationResponse, compNamesResponse] = await Promise.all([
      fetch('/api/trace?kind=Simulation'),
      fetch('/api/compnames')
    ]);
    
    const simulation = await simulationResponse.json();
    const componentNames = await compNamesResponse.json();
    
    if (simulation[0]) {
      this._startTime = simulation[0].start_time;
      this._endTime = simulation[0].end_time;
    }
    
    this._componentNames = componentNames.slice(0, 3);
    return { simulation: simulation[0], componentNames: this._componentNames };
  }

  async render(): Promise<void> {
    const content = this.createWidgetFrame();
    
    // Add expand button to header
    const header = this._container.querySelector('.widget-header') as HTMLDivElement;
    const expandBtn = document.createElement('button');
    expandBtn.classList.add('btn', 'btn-sm', 'btn-outline-primary', 'expand-btn');
    expandBtn.innerHTML = '<i class="fas fa-expand-arrows-alt"></i>';
    expandBtn.title = 'Open Full Dashboard';
    expandBtn.onclick = () => {
      window.location.href = '/dashboard';
    };
    header.appendChild(expandBtn);

    this.showLoading(content);

    try {
      const data = await this.getData();
      this.hideLoading();
      
      if (!data.simulation || this._componentNames.length === 0) {
        content.innerHTML = '<div class="no-data">No dashboard data available</div>';
        return;
      }

      content.innerHTML = `
        <div class="dashboard-preview">
          <div class="preview-widgets-container">
            <div class="preview-widget-column">
              <div class="preview-widget" id="preview-widget-0"></div>
              <div class="preview-widget" id="preview-widget-1"></div>
              <div class="preview-widget" id="preview-widget-2"></div>
            </div>
          </div>
          <div class="preview-footer">
            <small class="text-muted">
              Showing ${this._componentNames.length} components • 
              <a href="/dashboard" class="preview-link">View full dashboard →</a>
            </small>
          </div>
        </div>
      `;

      this.renderPreviewWidgets();
    } catch (error) {
      this.hideLoading();
      this.showError(content, 'Failed to load dashboard preview');
    }
  }

  private async renderPreviewWidgets(): Promise<void> {
    this._previewWidgets = [];
    
    for (let i = 0; i < this._componentNames.length; i++) {
      const widgetContainer = document.getElementById(`preview-widget-${i}`) as HTMLDivElement;
      if (!widgetContainer) continue;

      const componentName = this._componentNames[i];
      
      // Create component header manually to preserve original styling
      const componentHeader = document.createElement('div');
      componentHeader.classList.add('component-header');
      componentHeader.innerHTML = `
        <div class="title-bar">
          <h6>${componentName}</h6>
          <div class="btn"><i class="far fa-save"></i></div>
        </div>
      `;
      widgetContainer.appendChild(componentHeader);
      
      // Create widget content container
      const widgetContent = document.createElement('div');
      widgetContent.classList.add('widget-content');
      widgetContent.style.height = '120px'; // More space for better chart visibility
      widgetContainer.appendChild(widgetContent);
      
      const widget = new Widget(componentName, widgetContent, null as any);
      
      // Create a mini widget with adjusted dimensions for header
      const miniWidgetDiv = widget.createWidget(280, 120);
      miniWidgetDiv.classList.add('mini-dashboard-widget');
      
      // Set time range and axes
      widget.setXAxis(this._startTime, this._endTime);
      widget.setFirstAxis('ReqInCount');
      widget.setSecondAxis('AvgLatency');
      
      // Render the widget
      widget.render(true);
      
      // Add click handler to navigate to component view (on the whole container)
      widgetContainer.onclick = () => {
        window.location.href = `/component?name=${componentName}&starttime=${this._startTime}&endtime=${this._endTime}`;
      };
      widgetContainer.style.cursor = 'pointer';
      
      this._previewWidgets.push(widget);
    }
  }

  public resize(): void {
    this._previewWidgets.forEach(widget => {
      widget.resize(280, 120); // Adjusted for header space
    });
  }

  public destroy(): void {
    this._previewWidgets.forEach(widget => {
      if (widget.clear) {
        widget.clear();
      }
    });
    this._previewWidgets = [];
    super.destroy();
  }
}