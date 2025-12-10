import { WidgetBase, WidgetConfig } from './widget-base';
import * as d3 from 'd3';

interface BlockingTimeData {
  time: number;
  milestone_count: number;
}

export class BlockingAnalysisTimeWidget extends WidgetBase {
  constructor(container: HTMLDivElement, config: WidgetConfig) {
    super(container, config);
  }

  async getData(): Promise<BlockingTimeData[]> {
    try {
      const simulationResponse = await fetch('/api/trace?kind=Simulation');
      const simulation = await simulationResponse.json();
      
      if (!simulation || !simulation[0]) {
        console.log('No simulation data available');
        return this.getDefaultData();
      }

      const startTime = simulation[0].start_time;
      const endTime = simulation[0].end_time;
      
      console.log(`Milestone Analysis - Start: ${startTime}, End: ${endTime}`);
      
      // Use the dedicated milestones API for efficient milestone counting
      try {
        const milestoneParams = new URLSearchParams({
          start_time: startTime.toString(),
          end_time: endTime.toString(),
          num_windows: '10'
        });
        
        console.log('Fetching milestones from:', `/api/milestones?${milestoneParams.toString()}`);
        
        const milestonesResponse = await fetch(`/api/milestones?${milestoneParams.toString()}`);
        
        if (!milestonesResponse.ok) {
          console.error('Failed to fetch milestones:', milestonesResponse.status, milestonesResponse.statusText);
          return this.getDefaultData(endTime - startTime);
        }
        
        const milestoneData = await milestonesResponse.json();
        console.log('Milestone data received:', milestoneData);
        
        if (!milestoneData || !Array.isArray(milestoneData)) {
          console.error('Invalid milestone data format');
          return this.getDefaultData(endTime - startTime);
        }
        
        // Convert to our expected format
        const timeData: BlockingTimeData[] = milestoneData.map((item: any) => ({
          time: item.time,
          milestone_count: item.milestone_count
        }));
        
        console.log('Final milestone timeData:', timeData);
        return timeData;
        
      } catch (error) {
        console.error('Failed to fetch milestone data:', error);
        return this.getDefaultData(endTime - startTime);
      }
      
    } catch (error) {
      console.error('Failed to fetch blocking analysis time data:', error);
      return this.getDefaultData();
    }
  }

  private getDefaultData(duration: number = 1): BlockingTimeData[] {
    const defaultData: BlockingTimeData[] = [];
    for (let i = 0; i < 10; i++) {
      defaultData.push({
        time: (i * duration) / 10,
        milestone_count: 0
      });
    }
    return defaultData;
  }


  async render(): Promise<void> {
    const content = this.createWidgetFrame();
    this.showLoading(content);

    try {
      const data = await this.getData();
      this.hideLoading();
      
      // Always render the chart even if data is empty or all zeros
      content.innerHTML = '<div class="blocking-chart-container"><svg id="blocking-time-chart"></svg></div>';
      
      this.renderChart(data);
    } catch (error) {
      this.hideLoading();
      this.showError(content, 'Failed to load blocking analysis data');
    }
  }

  private renderChart(data: BlockingTimeData[]): void {
    const svg = d3.select('#blocking-time-chart');
    svg.selectAll("*").remove(); // Clear previous chart
    
    console.log('Rendering milestone chart with data:', data);
    
    const margin = { top: 10, right: 10, bottom: 35, left: 60 };
    const width = 280 - margin.left - margin.right;
    const height = 150 - margin.bottom - margin.top;

    svg.attr('width', width + margin.left + margin.right)
       .attr('height', height + margin.top + margin.bottom);

    const g = svg.append('g')
                 .attr('transform', `translate(${margin.left},${margin.top})`);

    // Ensure we have valid data
    if (!data || data.length === 0) {
      g.append('text')
       .attr('x', width / 2)
       .attr('y', height / 2)
       .attr('text-anchor', 'middle')
       .style('fill', '#999')
       .text('No milestone data available');
      return;
    }

    const timeExtent = d3.extent(data, d => d.time) as [number, number];
    const countExtent = d3.extent(data, d => d.milestone_count) as [number, number];
    
    console.log('Time extent:', timeExtent, 'Count extent:', countExtent);

    const xScale = d3.scaleLinear()
                     .domain(timeExtent)
                     .range([0, width]);

    const yScale = d3.scaleLinear()
                     .domain([0, Math.max(countExtent[1], 1)]) // Ensure max is at least 1
                     .range([height, 0]);

    // Add X axis with time formatting using SI prefix format
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

    // Add Y axis with integer format for milestone counts
    g.append('g')
     .call(d3.axisLeft(yScale).ticks(4).tickFormat(d3.format('d')));

    // Add axis labels
    g.append('text')
     .attr('transform', 'rotate(-90)')
     .attr('y', 0 - 60)
     .attr('x', 0 - (height / 2))
     .attr('dy', '1em')
     .style('text-anchor', 'middle')
     .style('font-size', '11px')
     .style('fill', '#666')
     .text('Milestones');

    g.append('text')
     .attr('transform', `translate(${width / 2}, ${height + 25})`)
     .style('text-anchor', 'middle')
     .style('font-size', '11px')
     .style('fill', '#666')
     .text('Time');

    const area = d3.area<BlockingTimeData>()
                   .x(d => xScale(d.time))
                   .y0(height)
                   .y1(d => yScale(d.milestone_count))
                   .curve(d3.curveMonotoneX);

    g.append('path')
     .datum(data)
     .attr('fill', '#28a745')
     .attr('fill-opacity', 0.6)
     .attr('stroke', '#28a745')
     .attr('stroke-width', 2)
     .attr('d', area);

    // Add dots for better visibility
    g.selectAll('.milestone-dot')
     .data(data)
     .enter().append('circle')
     .attr('class', 'milestone-dot')
     .attr('cx', d => xScale(d.time))
     .attr('cy', d => yScale(d.milestone_count))
     .attr('r', 3)
     .attr('fill', '#28a745')
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
       return `Time: ${timeStr}\nMilestones: ${d.milestone_count}`;
     });
  }
}