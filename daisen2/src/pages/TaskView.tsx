import React, { useEffect, useState } from 'react';
import { useSearchParams, useNavigate } from 'react-router-dom';
import { useTask, useSimulations } from '../hooks/useApi';
import TimelineVisualization from '../components/TimelineVisualization';

interface TaskViewProps {
  onMouseTimeUpdate: (time: string) => void;
}

const TaskView: React.FC<TaskViewProps> = ({ onMouseTimeUpdate }) => {
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const [startTime, setStartTime] = useState<number | undefined>();
  const [endTime, setEndTime] = useState<number | undefined>();
  
  const taskId = searchParams.get('id');
  const startTimeParam = searchParams.get('starttime');
  const endTimeParam = searchParams.get('endtime');
  
  const { task, loading: taskLoading, error: taskError } = useTask(taskId);
  const { simulations, loading: simLoading } = useSimulations();

  useEffect(() => {
    // If no task ID provided, redirect to simulation root task
    if (!taskId && simulations.length > 0) {
      const simulation = simulations[0];
      navigate(`/task?id=${simulation.id}`);
      return;
    }

    // Set time range from URL params or task data
    if (startTimeParam && endTimeParam) {
      setStartTime(parseFloat(startTimeParam));
      setEndTime(parseFloat(endTimeParam));
    } else if (task) {
      setStartTime(task.start_time);
      setEndTime(task.end_time);
    }
  }, [taskId, simulations, task, startTimeParam, endTimeParam, navigate]);

  const handleTimeRangeChange = (newStartTime: number, newEndTime: number) => {
    setStartTime(newStartTime);
    setEndTime(newEndTime);
    
    // Update URL with new time range
    const newParams = new URLSearchParams(searchParams);
    newParams.set('starttime', newStartTime.toString());
    newParams.set('endtime', newEndTime.toString());
    navigate(`/task?${newParams.toString()}`, { replace: true });
  };

  if (taskLoading || simLoading) {
    return (
      <div className="d-flex justify-content-center align-items-center h-100">
        <div className="spinner-border" role="status">
          <span className="visually-hidden">Loading...</span>
        </div>
      </div>
    );
  }

  if (taskError) {
    return (
      <div className="alert alert-danger m-3">
        Error: {taskError}
      </div>
    );
  }

  if (!task) {
    return (
      <div className="alert alert-warning m-3">
        No task found. Please select a task to view.
      </div>
    );
  }

  return (
    <div className="h-100 d-flex flex-column">
      {/* Task Header */}
      <div className="p-3 border-bottom bg-light">
        <div className="row">
          <div className="col-md-8">
            <h5 className="mb-1">{task.name}</h5>
            <div className="text-muted">
              <small>
                <strong>ID:</strong> {task.id} | 
                <strong> What:</strong> {task.what} | 
                <strong> Where:</strong> {task.where}
              </small>
            </div>
          </div>
          <div className="col-md-4 text-end">
            <div className="text-muted">
              <small>
                <strong>Duration:</strong> {((task.end_time - task.start_time) / 1000).toFixed(2)}ms
              </small>
            </div>
            <div className="text-muted">
              <small>
                <strong>Time Range:</strong> {task.start_time.toFixed(0)} - {task.end_time.toFixed(0)}
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
                if (task) {
                  handleTimeRangeChange(task.start_time, task.end_time);
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
            taskId={task.id}
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

export default TaskView;