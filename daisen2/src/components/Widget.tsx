import React, { useEffect, useRef, useState } from 'react';
import * as d3 from 'd3';
import { ApiService } from '../services/api';
import { DataObject, TimeValue } from '../types';

interface WidgetProps {
  componentName: string;
  startTime?: number;
  endTime?: number;
}

const Widget: React.FC<WidgetProps> = ({ componentName, startTime, endTime }) => {
  const svgRef = useRef<SVGSVGElement>(null);
  const [data, setData] = useState<DataObject[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const fetchData = async () => {
      try {
        setLoading(true);
        setError(null);
        
        // Try to fetch component data, fall back to mock data if API doesn't exist
        try {
          const componentData = await ApiService.getComponentData(componentName, startTime, endTime);
          setData(componentData);
        } catch (apiError) {
          // Generate mock data for demonstration
          const mockData: DataObject[] = [
            {
              info_type: 'throughput',
              data: generateMockTimeSeries(startTime || 0, endTime || 1000000, 50)
            }
          ];
          setData(mockData);
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to load widget data');
      } finally {
        setLoading(false);
      }
    };

    fetchData();
  }, [componentName, startTime, endTime]);

  useEffect(() => {
    if (!data.length || !svgRef.current || loading) return;

    renderChart();
  }, [data, loading]);

  const generateMockTimeSeries = (start: number, end: number, points: number): TimeValue[] => {
    const timeStep = (end - start) / points;
    const data: TimeValue[] = [];
    
    for (let i = 0; i < points; i++) {
      const time = start + i * timeStep;
      const value = Math.random() * 100 + Math.sin(i * 0.1) * 20 + 50;
      data.push({ time, value });
    }
    
    return data;
  };

  const renderChart = () => {
    const svg = d3.select(svgRef.current);
    svg.selectAll('*').remove();

    const container = svgRef.current?.parentElement;
    if (!container) return;

    const margin = { top: 10, right: 20, bottom: 30, left: 40 };
    const width = container.clientWidth - margin.left - margin.right;
    const height = 150 - margin.top - margin.bottom;

    const chartData = data[0]?.data || [];
    if (!chartData.length) return;

    const xScale = d3.scaleLinear()
      .domain(d3.extent(chartData, d => d.time) as [number, number])
      .range([0, width]);

    const yScale = d3.scaleLinear()
      .domain(d3.extent(chartData, d => d.value) as [number, number])
      .range([height, 0]);

    const line = d3.line<TimeValue>()
      .x(d => xScale(d.time))
      .y(d => yScale(d.value))
      .curve(d3.curveMonotoneX);

    const g = svg
      .attr('width', width + margin.left + margin.right)
      .attr('height', height + margin.top + margin.bottom)
      .append('g')
      .attr('transform', `translate(${margin.left},${margin.top})`);

    // Add X axis
    g.append('g')
      .attr('transform', `translate(0,${height})`)
      .call(d3.axisBottom(xScale).ticks(5).tickFormat(d3.format('.2s')));

    // Add Y axis  
    g.append('g')
      .call(d3.axisLeft(yScale).ticks(5).tickFormat(d3.format('.2s')));

    // Add the line
    g.append('path')
      .datum(chartData)
      .attr('fill', 'none')
      .attr('stroke', '#007bff')
      .attr('stroke-width', 2)
      .attr('d', line);

    // Add dots
    g.selectAll('.dot')
      .data(chartData.filter((_, i) => i % Math.max(1, Math.floor(chartData.length / 20)) === 0))
      .enter().append('circle')
      .attr('class', 'dot')
      .attr('cx', d => xScale(d.time))
      .attr('cy', d => yScale(d.value))
      .attr('r', 2)
      .attr('fill', '#007bff');
  };

  if (loading) {
    return (
      <div className="d-flex justify-content-center align-items-center h-100">
        <div className="spinner-border spinner-border-sm" role="status">
          <span className="visually-hidden">Loading...</span>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="text-center text-muted p-3">
        <small>No data available</small>
      </div>
    );
  }

  return (
    <div className="widget-chart">
      <svg ref={svgRef} className="w-100"></svg>
      {data.length > 0 && (
        <div className="mt-2">
          <small className="text-muted">
            Data type: {data[0].info_type} ({data[0].data.length} points)
          </small>
        </div>
      )}
    </div>
  );
};

export default Widget;