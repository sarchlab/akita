import { useState, useEffect } from 'react';
import { ApiService } from '../services/api';
import { SimulationInfo, Task } from '../types';

export function useSimulations() {
  const [simulations, setSimulations] = useState<SimulationInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const fetchSimulations = async () => {
      try {
        setLoading(true);
        const data = await ApiService.getSimulations();
        setSimulations(data);
        setError(null);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to fetch simulations');
      } finally {
        setLoading(false);
      }
    };

    fetchSimulations();
  }, []);

  return { simulations, loading, error };
}

export function useComponentNames() {
  const [componentNames, setComponentNames] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const fetchComponentNames = async () => {
      try {
        setLoading(true);
        const data = await ApiService.getComponentNames();
        setComponentNames(data);
        setError(null);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to fetch component names');
      } finally {
        setLoading(false);
      }
    };

    fetchComponentNames();
  }, []);

  const refetch = () => {
    const fetchComponentNames = async () => {
      try {
        setLoading(true);
        const data = await ApiService.getComponentNames();
        setComponentNames(data);
        setError(null);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to fetch component names');
      } finally {
        setLoading(false);
      }
    };
    fetchComponentNames();
  };

  return { componentNames, loading, error, refetch };
}

export function useTask(taskId: string | null) {
  const [task, setTask] = useState<Task | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!taskId) return;

    const fetchTask = async () => {
      try {
        setLoading(true);
        const data = await ApiService.getTaskById(taskId);
        setTask(data);
        setError(null);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to fetch task');
      } finally {
        setLoading(false);
      }
    };

    fetchTask();
  }, [taskId]);

  return { task, loading, error };
}

export function useComponentTasks(
  componentName: string | null,
  startTime?: number,
  endTime?: number
) {
  const [tasks, setTasks] = useState<Task[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!componentName) return;

    const fetchTasks = async () => {
      try {
        setLoading(true);
        const data = await ApiService.getComponentTasks(componentName, startTime, endTime);
        setTasks(data);
        setError(null);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to fetch component tasks');
      } finally {
        setLoading(false);
      }
    };

    fetchTasks();
  }, [componentName, startTime, endTime]);

  return { tasks, loading, error, refetch: () => {} };
}