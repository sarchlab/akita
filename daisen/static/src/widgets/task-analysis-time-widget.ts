import { WidgetBase, WidgetConfig } from './widget-base';
import * as d3 from 'd3';

interface TimeTaskData {
  time: number;
  count: number;
}

export class TaskAnalysisTimeWidget extends WidgetBase {
  constructor(container: HTMLDivElement, config: WidgetConfig) {
    super(container, config);
  }

  async getData(): Promise<TimeTaskData[]> {
    try {
      const simulationResponse = await fetch('/api/trace?kind=Simulation');
      const simulation = await simulationResponse.json();
      
      if (!simulation || !simulation[0]) {
        return [];
      }

      const startTime = simulation[0].start_time;
      const endTime = simulation[0].end_time;
      const duration = endTime - startTime;
      const timeStep = duration / 15; // 15 time points for good resolution
      
      console.log(`Task Analysis Time - Start: ${startTime}, End: ${endTime}, Duration: ${duration}, Step: ${timeStep}`);
      
      const compNamesResponse = await fetch('/api/compnames');
      const componentNames = await compNamesResponse.json();
      
      if (!componentNames || componentNames.length === 0) {
        return [];
      }
      
      const timeData: TimeTaskData[] = [];
      
      // Get task count data for each time point
      for (let i = 0; i < 15; i++) {
        const currentTime = startTime + (i * timeStep);
        let totalTaskCount = 0;
        
        // Aggregate task counts from all components at this time point
        const promises = componentNames.slice(0, 10).map(async (comp: string) => {
          try {
            const params = new URLSearchParams({
              info_type: 'ConcurrentTask',
              where: comp,
              start_time: currentTime.toString(),
              end_time: (currentTime + timeStep).toString(),
              num_dots: '3'
            });
            
            const response = await fetch(`/api/compinfo?${params.toString()}`);
            const data = await response.json();
            
            if (data && data.data && Array.isArray(data.data)) {
              // Sum up all task values in this time window
              return data.data.reduce((sum: number, point: any) => {
                return sum + (point && typeof point.value === 'number' ? Math.round(point.value) : 0);
              }, 0);
            }
            return 0;
          } catch (error) {
            console.warn(`Failed to get task data for component ${comp} at time ${currentTime}:`, error);
            return 0;
          }
        });
        
        try {
          const componentTaskCounts = await Promise.all(promises);
          totalTaskCount = componentTaskCounts.reduce((sum, count) => sum + count, 0);
        } catch (error) {
          console.warn(`Failed to aggregate task counts for time ${currentTime}:`, error);
          totalTaskCount = 0;
        }
        
        // Use relative time from start for better display
        const relativeTime = currentTime - startTime;
        
        timeData.push({
          time: relativeTime,
          count: totalTaskCount
        });
        
        console.log(`Time point ${i}: abs=${currentTime}, rel=${relativeTime}, Tasks: ${totalTaskCount}`);
      }
      
      return timeData;
    } catch (error) {
      console.error('Failed to fetch task analysis time data:', error);
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
        content.innerHTML = '<div class="no-data">No time analysis data available</div>';
        return;
      }

      content.innerHTML = '<div class="time-chart-container"><svg id="time-chart"></svg></div>';
      
      this.renderChart(data);
    } catch (error) {
      this.hideLoading();
      this.showError(content, 'Failed to load time analysis data');
    }
  }

  private renderChart(data: TimeTaskData[]): void {
    const svg = d3.select('#time-chart');
    svg.selectAll("*").remove(); // Clear previous chart
    
    const margin = { top: 10, right: 10, bottom: 35, left: 45 };
    const width = 280 - margin.left - margin.right;
    const height = 150 - margin.bottom - margin.top;

    svg.attr('width', width + margin.left + margin.right)
       .attr('height', height + margin.top + margin.bottom);

    const g = svg.append('g')
                 .attr('transform', `translate(${margin.left},${margin.top})`);

    // Ensure we have data and valid extent
    if (!data || data.length === 0) {
      g.append('text')
       .attr('x', width / 2)
       .attr('y', height / 2)
       .attr('text-anchor', 'middle')
       .style('fill', '#999')
       .text('No data available');
      return;
    }

    const timeExtent = d3.extent(data, d => d.time) as [number, number];
    const countExtent = d3.extent(data, d => d.count) as [number, number];
    
    const xScale = d3.scaleLinear()
                     .domain(timeExtent)
                     .range([0, width]);

    const yScale = d3.scaleLinear()
                     .domain([0, Math.max(countExtent[1], 1)]) // Ensure max is at least 1
                     .range([height, 0]);

    // Calculate bar width
    const barWidth = Math.max(1, (width / data.length) * 0.8);

    // Add X axis with SI prefix formatting
    g.append('g')
     .attr('transform', `translate(0,${height})`)
     .call(d3.axisBottom(xScale)
           .ticks(5)
           .tickFormat(d => {
             const timeValue = Number(d);
             if (timeValue === 0) {
               return '0s';
             } else {
               return d3.format('.3s')(timeValue) + 's';
             }
           }));

    // Add Y axis  
    g.append('g')
     .call(d3.axisLeft(yScale)
           .ticks(4)
           .tickFormat(d3.format('d'))); // Integer format for task counts

    // Add axis labels
    g.append('text')
     .attr('transform', 'rotate(-90)')
     .attr('y', 0 - margin.left)
     .attr('x', 0 - (height / 2))
     .attr('dy', '1em')
     .style('text-anchor', 'middle')
     .style('font-size', '11px')
     .style('fill', '#666')
     .text('Tasks');

    g.append('text')
     .attr('transform', `translate(${width / 2}, ${height + margin.bottom - 5})`)
     .style('text-anchor', 'middle')
     .style('font-size', '11px')
     .style('fill', '#666')
     .text('Time (from start)');

    // Add bars
    g.selectAll('.bar')
     .data(data)
     .enter().append('rect')
     .attr('class', 'bar')
     .attr('x', d => xScale(d.time) - barWidth / 2)
     .attr('y', d => yScale(d.count))
     .attr('width', barWidth)
     .attr('height', d => height - yScale(d.count))
     .attr('fill', '#007bff')
     .attr('stroke', '#0056b3')
     .attr('stroke-width', 0.5)
     .on('mouseover', function(event, d) {
       d3.select(this).attr('fill', '#0056b3');
     })
     .on('mouseout', function(event, d) {
       d3.select(this).attr('fill', '#007bff');
     })
     .append('title')
     .text(d => {
       let timeStr;
       if (d.time >= 1) {
         timeStr = d.time.toFixed(3) + 's';
       } else if (d.time >= 0.001) {
         timeStr = d.time.toFixed(6) + 's';
       } else {
         timeStr = d.time.toExponential(3) + 's';
       }
       return `Time: ${timeStr}\nTasks: ${d.count}`;
     });
  }
}