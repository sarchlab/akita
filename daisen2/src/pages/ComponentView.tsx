import React, { useEffect, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { useComponentTasks, useSimulations } from '../hooks/useApi';
import TimelineVisualization from '../components/TimelineVisualization';

interface ComponentViewProps {
  onMouseTimeUpdate: (time: string) => void;
}

const ComponentView: React.FC<ComponentViewProps> = ({ onMouseTimeUpdate }) => {
  const [searchParams] = useSearchParams();
  const [startTime, setStartTime] = useState<number | undefined>();
  const [endTime, setEndTime] = useState<number | undefined>();
  
  const componentName = searchParams.get('name');
  const startTimeParam = searchParams.get('starttime');
  const endTimeParam = searchParams.get('endtime');
  
  const { simulations } = useSimulations();
  
  // Parse time range from URL or use simulation defaults
  useEffect(() => {
    if (startTimeParam && endTimeParam) {
      setStartTime(parseFloat(startTimeParam));
      setEndTime(parseFloat(endTimeParam));
    } else if (simulations.length > 0) {
      const simulation = simulations[0];
      setStartTime(simulation.start_time);
      setEndTime(simulation.end_time);
    }
  }, [startTimeParam, endTimeParam, simulations]);

  const { tasks, loading: tasksLoading, error: tasksError } = useComponentTasks(
    componentName,
    startTime,
    endTime
  );

  const handleTimeRangeChange = (newStartTime: number, newEndTime: number) => {
    setStartTime(newStartTime);
    setEndTime(newEndTime);
    
    // Update URL with new time range
    const newParams = new URLSearchParams(searchParams);
    newParams.set('starttime', newStartTime.toString());
    newParams.set('endtime', newEndTime.toString());
    window.history.replaceState(null, '', `/component?${newParams.toString()}`);
  };

  if (!componentName) {
    return (
      <div className="alert alert-warning m-3">
        No component specified. Please select a component to view.
      </div>
    );
  }

  if (tasksLoading) {
    return (
      <div className="d-flex justify-content-center align-items-center h-100">
        <div className="spinner-border" role="status">
          <span className="visually-hidden">Loading...</span>
        </div>
      </div>
    );
  }

  if (tasksError) {
    return (
      <div className="alert alert-danger m-3">
        Error: {tasksError}
      </div>
    );
  }

  return (
    <div className="h-100 d-flex flex-column">
      {/* Component Header */}
      <div className="p-3 border-bottom bg-light">
        <div className="row">
          <div className="col-md-8">
            <h5 className="mb-1">Component: {componentName}</h5>
            <div className="text-muted">
              <small>
                <strong>Tasks:</strong> {tasks.length} | 
                <strong> Time Range:</strong> {startTime?.toFixed(0)} - {endTime?.toFixed(0)}
              </small>
            </div>
          </div>
          <div className="col-md-4 text-end">
            <div className="text-muted">
              <small>
                <strong>Duration:</strong> {
                  startTime && endTime ? 
                    ((endTime - startTime) / 1000).toFixed(2) + 'ms' : 
                    'N/A'
                }
              </small>
            </div>
          </div>
        </div>
      </div>

      {/* Timeline Controls */}
      <div className="p-2 border-bottom">
        <div className="row align-items-center">
          <div className="col-md-4">
            <label className="form-label mb-0 me-2">Start Time:</label>
            <input
              type="number"
              className="form-control form-control-sm d-inline-block"
              style={{ width: '120px' }}
              value={startTime || ''}
              onChange={(e) => {
                const value = parseFloat(e.target.value);
                if (!isNaN(value) && endTime) {
                  handleTimeRangeChange(value, endTime);
                }
              }}
            />
          </div>
          <div className="col-md-4">
            <label className="form-label mb-0 me-2">End Time:</label>
            <input
              type="number"
              className="form-control form-control-sm d-inline-block"
              style={{ width: '120px' }}
              value={endTime || ''}
              onChange={(e) => {
                const value = parseFloat(e.target.value);
                if (!isNaN(value) && startTime) {
                  handleTimeRangeChange(startTime, value);
                }
              }}
            />
          </div>
          <div className="col-md-4 text-end">
            <button
              className="btn btn-sm btn-outline-secondary"
              onClick={() => {
                if (simulations.length > 0) {
                  const simulation = simulations[0];
                  handleTimeRangeChange(simulation.start_time, simulation.end_time);
                }
              }}
            >
              Reset Zoom
            </button>
          </div>
        </div>
      </div>

      {/* Timeline Visualization */}
      <div className="flex-grow-1 overflow-hidden">
        {startTime !== undefined && endTime !== undefined && (
          <TimelineVisualization
            componentName={componentName}
            tasks={tasks}
            startTime={startTime}
            endTime={endTime}
            onTimeRangeChange={handleTimeRangeChange}
            onMouseTimeUpdate={onMouseTimeUpdate}
          />
        )}
      </div>
    </div>
  );
};

export default ComponentView;