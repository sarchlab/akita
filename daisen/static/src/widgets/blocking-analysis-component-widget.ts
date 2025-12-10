import { WidgetBase, WidgetConfig } from './widget-base';

interface ComponentBlockingData {
  component: string;
  milestone_count: number;
  avg_milestones_per_task: number;
  total_tasks: number;
}

export class BlockingAnalysisComponentWidget extends WidgetBase {
  constructor(container: HTMLDivElement, config: WidgetConfig) {
    super(container, config);
  }

  async getData(): Promise<ComponentBlockingData[]> {
    const simulationResponse = await fetch('/api/trace?kind=Simulation');
    const simulation = await simulationResponse.json();
    
    if (!simulation[0]) {
      return [];
    }

    const startTime = simulation[0].start_time;
    const endTime = simulation[0].end_time;
    
    console.log(`Blocking Analysis Component - Getting milestone counts from ${startTime} to ${endTime}`);
    
    try {
      // Use new component_milestones API to get milestone counts by component
      const milestoneParams = new URLSearchParams({
        start_time: startTime.toString(),
        end_time: endTime.toString()
      });
      
      const milestoneResponse = await fetch(`/api/component_milestones?${milestoneParams.toString()}`);
      const milestoneData = await milestoneResponse.json();
      
      console.log('Component milestone data from API:', milestoneData);
      
      if (!milestoneData || !Array.isArray(milestoneData)) {
        return [];
      }
      
      // Get task counts for each component with milestones
      const blockingData = await Promise.all(
        milestoneData.map(async (item: any) => {
          const comp = item.component;
          const milestoneCount = item.milestone_count;
          
          try {
            // Get task count for this component
            const taskParams = new URLSearchParams({
              where: comp,
              starttime: startTime.toString(),
              endtime: endTime.toString()
            });
            
            const taskResponse = await fetch(`/api/trace?${taskParams.toString()}`);
            const tasks = await taskResponse.json();
            const totalTasks = tasks && Array.isArray(tasks) ? tasks.length : 0;
            
            const avgMilestonesPerTask = totalTasks > 0 ? milestoneCount / totalTasks : 0;
            
            console.log(`Component ${comp}: ${totalTasks} tasks, ${milestoneCount} milestones`);
            
            return {
              component: comp,
              milestone_count: milestoneCount,
              avg_milestones_per_task: avgMilestonesPerTask,
              total_tasks: totalTasks
            };
          } catch (error) {
            console.warn(`Failed to get task data for component ${comp}:`, error);
            return {
              component: comp,
              milestone_count: milestoneCount,
              avg_milestones_per_task: 0,
              total_tasks: 0
            };
          }
        })
      );
      
      const filteredData = blockingData.filter(item => item.milestone_count > 0);
      console.log('Final blocking analysis component data:', filteredData);
      
      return filteredData;
      
    } catch (error) {
      console.error('Failed to fetch component milestone data:', error);
      return [];
    }
  }

  async render(): Promise<void> {
    const content = this.createWidgetFrame();
    this.showLoading(content);

    try {
      const data = await this.getData();
      this.hideLoading();
      
      if (data.length === 0) {
        content.innerHTML = '<div class="no-data">No milestone data available</div>';
        return;
      }

      const maxMilestones = Math.max(...data.map(d => d.milestone_count));
      
      content.innerHTML = `
        <div class="blocking-analysis-component">
          ${data.map(item => `
            <div class="blocking-row" onclick="window.location.href='/component?name=${item.component}'">
              <div class="blocking-info">
                <span class="component-name">${item.component}</span>
                <div class="blocking-stats">
                  <span class="blocking-count">${item.milestone_count} milestones</span>
                  <span class="avg-milestones">Avg per task: ${item.avg_milestones_per_task.toFixed(1)}</span>
                  <span class="total-tasks">${item.total_tasks} tasks</span>
                </div>
              </div>
              <div class="blocking-bar">
                <div class="blocking-bar-fill" style="width: ${maxMilestones > 0 ? (item.milestone_count / maxMilestones) * 100 : 0}%"></div>
              </div>
            </div>
          `).join('')}
        </div>
      `;
    } catch (error) {
      this.hideLoading();
      this.showError(content, 'Failed to load milestone analysis data');
    }
  }
}