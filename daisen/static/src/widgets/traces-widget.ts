import { WidgetBase, WidgetConfig } from './widget-base';

interface TraceData {
  name: string;
  start_time: string;
  end_time: string;
}

export class TracesWidget extends WidgetBase {
  constructor(container: HTMLDivElement, config: WidgetConfig) {
    super(container, config);
  }

  async getData(): Promise<TraceData[]> {
    try {
      const response = await fetch('/api/exec_info');
      const data = await response.json();
      
      if (!data) {
        return [];
      }
      
      // Return single simulation entry from exec_info
      return [{
        name: 'Simulation 1',
        start_time: data.start_time || 'Unknown',
        end_time: data.end_time || 'Unknown'
      }];
    } catch (error) {
      console.error('Failed to fetch exec_info:', error);
      return [];
    }
  }

  async render(): Promise<void> {
    const content = this.createWidgetFrame();
    this.showLoading(content);

    try {
      const traces = await this.getData();
      this.hideLoading();
      
      if (traces.length === 0) {
        content.innerHTML = '<div class="no-data">No traces available</div>';
        return;
      }

      content.innerHTML = `
        <div class="traces-list">
          ${traces.map(trace => `
            <div class="trace-item">
              <div class="trace-header">
                <span class="trace-name">${trace.name}</span>
              </div>
              <div class="trace-details">
                <span class="trace-time">
                  <i class="fas fa-clock"></i>
                  ${this.formatTime(trace.start_time)} - ${this.formatTime(trace.end_time)}
                </span>
              </div>
            </div>
          `).join('')}
        </div>
      `;
    } catch (error) {
      this.hideLoading();
      this.showError(content, 'Failed to load traces');
    }
  }

  private formatTime(timestamp: string): string {
    if (!timestamp || timestamp === 'Unknown') {
      return 'Unknown';
    }
    
    try {
      const date = new Date(timestamp);
      const month = (date.getMonth() + 1).toString().padStart(2, '0');
      const day = date.getDate().toString().padStart(2, '0');
      const year = date.getFullYear();
      const hours = date.getHours().toString().padStart(2, '0');
      const minutes = date.getMinutes().toString().padStart(2, '0');
      const seconds = date.getSeconds().toString().padStart(2, '0');
      
      return `${month}/${day}/${year} ${hours}:${minutes}:${seconds}`;
    } catch (error) {
      return timestamp;
    }
  }
}