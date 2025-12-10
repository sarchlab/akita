import { WidgetBase, WidgetConfig } from './widget-base';

interface SimulationData {
  id: string;
  start_time: string;
  end_time: string;
  duration: string;
  status: string;
  command: string;
  working_directory: string;
}

export class SimulationInfoWidget extends WidgetBase {
  constructor(container: HTMLDivElement, config: WidgetConfig) {
    super(container, config);
  }

  async getData(): Promise<SimulationData> {
    try {
      // Get simulation data from trace for the ID
      const simResponse = await fetch('/api/trace?kind=Simulation');
      const simData = await simResponse.json();
      
      // Get exec_info data for command and real times
      const execResponse = await fetch('/api/exec_info');
      const execData = await execResponse.json();
      
      if (simData && simData[0]) {
        const sim = simData[0];
        
        // Calculate duration from exec_info times
        let duration = 'Unknown';
        if (execData.start_time && execData.end_time) {
          const startDate = new Date(execData.start_time);
          const endDate = new Date(execData.end_time);
          const durationMs = endDate.getTime() - startDate.getTime();
          duration = `${(durationMs / 1000).toFixed(3)} seconds`;
        }
        
        return {
          id: sim.id,
          start_time: execData.start_time || 'Unknown',
          end_time: execData.end_time || 'Unknown',
          duration: duration,
          status: 'completed',
          command: execData.command || './daisen -sqlite unknown.db -http=:3001',
          working_directory: execData.working_directory || 'Unknown'
        };
      }
    } catch (error) {
      console.error('Error fetching simulation data:', error);
    }
    
    throw new Error('No simulation data available');
  }

  private formatTime(timestamp: string): string {
    // Format the real timestamp from exec_info
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
      return timestamp; // Return original if parsing fails
    }
  }

  async render(): Promise<void> {
    const content = this.createWidgetFrame();
    this.showLoading(content);

    try {
      const data = await this.getData();
      this.hideLoading();
      
      content.innerHTML = `
        <div class="simulation-info">
          <div class="info-row">
            <span class="info-label">Command:</span>
            <span class="info-value" style="font-family: monospace; font-size: 11px;">${data.command}</span>
          </div>
          <div class="info-row">
            <span class="info-label">Start Time:</span>
            <span class="info-value">${this.formatTime(data.start_time)}</span>
          </div>
          <div class="info-row">
            <span class="info-label">End Time:</span>
            <span class="info-value">${this.formatTime(data.end_time)}</span>
          </div>
          <div class="info-row">
            <span class="info-label">Duration:</span>
            <span class="info-value">${data.duration}</span>
          </div>
          <div class="info-row">
            <span class="info-label">Status:</span>
            <span class="info-value status-${data.status}">
              <i class="fas fa-check-circle"></i> ${data.status}
            </span>
          </div>
        </div>
      `;
    } catch (error) {
      this.hideLoading();
      this.showError(content, 'Failed to load simulation information');
    }
  }
}