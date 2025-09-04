import React, { useRef, useEffect, useState, useCallback } from 'react';
import * as d3 from 'd3';
import { Task } from '../types';

interface TimelineVisualizationProps {
  taskId?: string;
  componentName?: string;
  tasks?: Task[];
  startTime: number;
  endTime: number;
  onTimeRangeChange: (startTime: number, endTime: number) => void;
  onMouseTimeUpdate: (time: string) => void;
}

const TimelineVisualization: React.FC<TimelineVisualizationProps> = ({
  taskId,
  tasks: propTasks,
  startTime,
  endTime,
  onTimeRangeChange,
  onMouseTimeUpdate,
}) => {
  const svgRef = useRef<SVGSVGElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const [tasks, setTasks] = useState<Task[]>(propTasks || []);
  const [loading, setLoading] = useState(false);

  // Fetch tasks if taskId is provided
  useEffect(() => {
    if (taskId && !propTasks) {
      const fetchTasks = async () => {
        setLoading(true);
        try {
          // In a real implementation, this would fetch child tasks
          // For now, create a mock task to demonstrate
          const mockTask: Task = {
            id: taskId,
            parent_id: '',
            name: `Task ${taskId}`,
            what: 'Sample Task',
            where: 'Component',
            start_time: startTime,
            end_time: endTime,
            level: 0,
            dim: {
              x: 0,
              y: 0,
              width: 100,
              height: 20,
              startTime,
              endTime
            }
          };
          setTasks([mockTask]);
        } catch (error) {
          console.error('Failed to fetch tasks:', error);
        } finally {
          setLoading(false);
        }
      };

      fetchTasks();
    } else if (propTasks) {
      setTasks(propTasks);
    }
  }, [taskId, propTasks, startTime, endTime]);

  // Arrange tasks in a tree-like hierarchy
  const arrangeTasksAsTree = useCallback((tasks: Task[]) => {
    const taskMap = new Map<string, Task>();
    const roots: Task[] = [];
    
    // Build task map
    tasks.forEach(task => {
      taskMap.set(task.id, { ...task, level: 0 });
    });

    // Assign levels based on parent-child relationships
    tasks.forEach(task => {
      const taskCopy = taskMap.get(task.id)!;
      if (task.parent_id && taskMap.has(task.parent_id)) {
        const parent = taskMap.get(task.parent_id)!;
        taskCopy.level = parent.level + 1;
      } else {
        roots.push(taskCopy);
      }
    });

    return Array.from(taskMap.values()).sort((a, b) => a.level - b.level);
  }, []);

  // Render the timeline visualization
  const renderTimeline = useCallback(() => {
    if (!svgRef.current || !containerRef.current || tasks.length === 0) return;

    const svg = d3.select(svgRef.current);
    svg.selectAll('*').remove();

    const container = containerRef.current;
    const margin = { top: 20, right: 20, bottom: 40, left: 60 };
    const width = container.clientWidth - margin.left - margin.right;
    const height = Math.max(400, tasks.length * 25 + 100) - margin.top - margin.bottom;

    svg
      .attr('width', width + margin.left + margin.right)
      .attr('height', height + margin.top + margin.bottom);

    const g = svg.append('g')
      .attr('transform', `translate(${margin.left},${margin.top})`);

    // Arrange tasks in tree structure
    const arrangedTasks = arrangeTasksAsTree(tasks);

    // Create scales
    const xScale = d3.scaleLinear()
      .domain([startTime, endTime])
      .range([0, width]);

    const yScale = d3.scaleBand()
      .domain(arrangedTasks.map((_, i) => i.toString()))
      .range([0, height])
      .padding(0.1);

    // Color scale based on task level
    const colorScale = d3.scaleOrdinal<string>()
      .domain(['0', '1', '2', '3', '4'])
      .range(['#1f77b4', '#ff7f0e', '#2ca02c', '#d62728', '#9467bd']);

    // Add X axis
    const xAxis = g.append('g')
      .attr('transform', `translate(0,${height})`)
      .call(d3.axisBottom(xScale).tickFormat(d3.format('.2s')));

    // Add Y axis with task names
    g.append('g')
      .call(d3.axisLeft(d3.scaleBand()
        .domain(arrangedTasks.map(t => t.name.length > 15 ? t.name.substring(0, 15) + '...' : t.name))
        .range([0, height])
        .padding(0.1)
      ));

    // Add task bars
    const bars = g.selectAll('.task-bar')
      .data(arrangedTasks)
      .enter()
      .append('rect')
      .attr('class', 'task-bar')
      .attr('x', d => Math.max(0, xScale(d.start_time)))
      .attr('y', (_, i) => yScale(i.toString()) || 0)
      .attr('width', d => Math.max(1, xScale(d.end_time) - xScale(d.start_time)))
      .attr('height', yScale.bandwidth())
      .attr('fill', d => colorScale(d.level.toString()))
      .attr('stroke', '#333')
      .attr('stroke-width', 0.5)
      .style('cursor', 'pointer');

    // Add tooltips
    bars.append('title')
      .text(d => `${d.name}\nStart: ${d.start_time}\nEnd: ${d.end_time}\nDuration: ${d.end_time - d.start_time}`);

    // Add mouse tracking line
    const mouseLineGroup = g.append('g').style('display', 'none');
    const mouseLine = mouseLineGroup.append('line')
      .attr('stroke', '#666')
      .attr('stroke-width', 1)
      .attr('stroke-dasharray', '3,3')
      .attr('y1', 0)
      .attr('y2', height);

    // Mouse events for time tracking
    svg.append('rect')
      .attr('class', 'overlay')
      .attr('width', width + margin.left + margin.right)
      .attr('height', height + margin.top + margin.bottom)
      .style('fill', 'none')
      .style('pointer-events', 'all')
      .on('mousemove', function(event) {
        const [mouseX] = d3.pointer(event);
        const adjustedX = mouseX - margin.left;
        
        if (adjustedX >= 0 && adjustedX <= width) {
          const time = xScale.invert(adjustedX);
          onMouseTimeUpdate(time.toFixed(2));
          
          mouseLine.attr('x1', adjustedX).attr('x2', adjustedX);
          mouseLineGroup.style('display', null);
        }
      })
      .on('mouseleave', () => {
        mouseLineGroup.style('display', 'none');
        onMouseTimeUpdate('');
      });

    // Zoom and pan functionality
    const zoom = d3.zoom<SVGSVGElement, unknown>()
      .scaleExtent([0.1, 50])
      .on('zoom', (event) => {
        const { transform } = event;
        
        // Update x scale
        const newXScale = transform.rescaleX(xScale);
        
        // Update axis
        xAxis.call(d3.axisBottom(newXScale).tickFormat(d3.format('.2s') as any));
        
        // Update bars
        bars
          .attr('x', d => Math.max(0, newXScale(d.start_time)))
          .attr('width', d => Math.max(1, newXScale(d.end_time) - newXScale(d.start_time)));
      })
      .on('end', (event) => {
        const { transform } = event;
        const newXScale = transform.rescaleX(xScale);
        const [newStart, newEnd] = newXScale.domain();
        onTimeRangeChange(newStart, newEnd);
      });

    svg.call(zoom);

  }, [tasks, startTime, endTime, onTimeRangeChange, onMouseTimeUpdate, arrangeTasksAsTree]);

  // Re-render when dependencies change
  useEffect(() => {
    renderTimeline();
  }, [renderTimeline]);

  // Handle window resize
  useEffect(() => {
    const handleResize = () => {
      renderTimeline();
    };

    window.addEventListener('resize', handleResize);
    return () => window.removeEventListener('resize', handleResize);
  }, [renderTimeline]);

  if (loading) {
    return (
      <div className="d-flex justify-content-center align-items-center h-100">
        <div className="spinner-border" role="status">
          <span className="visually-hidden">Loading timeline...</span>
        </div>
      </div>
    );
  }

  return (
    <div ref={containerRef} className="visualization-container">
      <svg ref={svgRef} className="timeline-svg"></svg>
      {tasks.length === 0 && (
        <div className="position-absolute top-50 start-50 translate-middle text-center">
          <h6 className="text-muted">No tasks to display</h6>
          <p className="text-muted">Try adjusting the time range or selecting a different component.</p>
        </div>
      )}
    </div>
  );
};

export default TimelineVisualization;