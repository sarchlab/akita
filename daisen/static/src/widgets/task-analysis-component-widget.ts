import { WidgetBase, WidgetConfig } from './widget-base';

interface ComponentTaskData {
  component: string;
  task_count: number;
  total_duration: number;
}

export class TaskAnalysisComponentWidget extends WidgetBase {
  constructor(container: HTMLDivElement, config: WidgetConfig) {
    super(container, config);
  }

  async getData(): Promise<ComponentTaskData[]> {
    const [compNamesResponse, simulationResponse] = await Promise.all([
      fetch('/api/compnames'),
      fetch('/api/trace?kind=Simulation')
    ]);
    
    const componentNames = await compNamesResponse.json();
    const simulation = await simulationResponse.json();
    
    if (!simulation[0]) {
      return [];
    }

    const startTime = simulation[0].start_time;
    const endTime = simulation[0].end_time;
    
    console.log(`Task Analysis Component - Getting task counts for components from ${startTime} to ${endTime}`);
    
    const taskData = await Promise.all(
      componentNames.slice(0, 5).map(async (comp: string) => {
        try {
          // Get all tasks for this component using trace API
          const params = new URLSearchParams({
            where: comp,
            starttime: startTime.toString(),
            endtime: endTime.toString()
          });
          
          console.log(`Fetching tasks for component ${comp}: /api/trace?${params.toString()}`);
          
          const response = await fetch(`/api/trace?${params.toString()}`);
          const tasks = await response.json();
          
          if (tasks && Array.isArray(tasks)) {
            const taskCount = tasks.length;
            console.log(`Component ${comp}: found ${taskCount} tasks`);
            
            // Calculate total duration from actual task durations
            const totalDuration = tasks.reduce((sum: number, task: any) => {
              const duration = task.end_time - task.start_time;
              return sum + duration;
            }, 0);
            
            return {
              component: comp,
              task_count: taskCount,
              total_duration: totalDuration
            };
          }
        } catch (error) {
          console.warn(`Failed to get data for component ${comp}:`, error);
        }
        
        return {
          component: comp,
          task_count: 0,
          total_duration: 0
        };
      })
    );
    
    const sortedData = taskData.sort((a, b) => b.task_count - a.task_count);
    console.log('Final task analysis component data:', sortedData);
    
    return sortedData;
  }

  async render(): Promise<void> {
    const content = this.createWidgetFrame();
    this.showLoading(content);

    try {
      const data = await this.getData();
      this.hideLoading();
      
      if (data.length === 0) {
        content.innerHTML = '<div class="no-data">No task data available</div>';
        return;
      }

      const maxTasks = Math.max(...data.map(d => d.task_count));
      
      content.innerHTML = `
        <div class="task-analysis-component">
          ${data.map(item => `
            <div class="component-row" onclick="window.location.href='/component?name=${item.component}'">
              <div class="component-info">
                <span class="component-name">${item.component}</span>
                <div class="component-stats">
                  <span class="task-count">${item.task_count} tasks</span>
                </div>
              </div>
              <div class="task-bar">
                <div class="task-bar-fill" style="width: ${maxTasks > 0 ? (item.task_count / maxTasks) * 100 : 0}%"></div>
              </div>
            </div>
          `).join('')}
        </div>
      `;
    } catch (error) {
      this.hideLoading();
      this.showError(content, 'Failed to load task analysis data');
    }
  }
}