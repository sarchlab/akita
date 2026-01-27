import { WidgetBase, WidgetConfig } from './widget-base';

interface Segment {
  start_time: number;
  end_time: number;
}

interface SegmentsResponse {
  enabled: boolean;
  segments: Segment[];
}

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
      const response = await fetch('/api/segments');
      const data: SegmentsResponse = await response.json();

      if (!data || !data.enabled || !data.segments || data.segments.length === 0) {
        // Fallback to single simulation from exec_info
        const execInfoResponse = await fetch('/api/exec_info');
        const execInfo = await execInfoResponse.json();

        if (!execInfo) {
          return [];
        }

        return [{
          name: 'Simulation 1',
          start_time: execInfo.start_time || 'Unknown',
          end_time: execInfo.end_time || 'Unknown'
        }];
      }

      // Convert segments to trace data with simulation names
      return data.segments.map((segment, index) => ({
        name: `Simulation ${index + 1}`,
        start_time: segment.start_time.toString(),
        end_time: segment.end_time.toString()
      }));
    } catch (error) {
      console.error('Failed to fetch segments:', error);
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

    // Check if it's a date string (contains '-' or ':')
    if (timestamp.includes('-') || timestamp.includes(':')) {
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

    // Otherwise, treat as simulation time in seconds
    const timeValue = parseFloat(timestamp);
    if (!isNaN(timeValue)) {
      // Format as simulation time with appropriate unit
      if (timeValue < 0.000001) {
        return `${(timeValue * 1e9).toFixed(2)} ns`;
      } else if (timeValue < 0.001) {
        return `${(timeValue * 1e6).toFixed(2)} μs`;
      } else if (timeValue < 1) {
        return `${(timeValue * 1e3).toFixed(2)} ms`;
      } else {
        return `${timeValue.toFixed(2)} s`;
      }
    }

    return timestamp;
  }
}