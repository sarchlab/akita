// API service for communicating with the Go backend

import { SimulationInfo, Task, DataObject } from '../types';

const API_BASE = '/api';

export class ApiService {
  static async getSimulations(): Promise<SimulationInfo[]> {
    const response = await fetch(`${API_BASE}/trace?kind=Simulation`);
    if (!response.ok) {
      throw new Error('Failed to fetch simulations');
    }
    return response.json();
  }

  static async getTaskById(id: string): Promise<Task> {
    const response = await fetch(`${API_BASE}/trace?id=${id}`);
    if (!response.ok) {
      throw new Error(`Failed to fetch task ${id}`);
    }
    const data = await response.json();
    return data[0];
  }

  static async getComponentNames(): Promise<string[]> {
    const response = await fetch(`${API_BASE}/compnames`);
    if (!response.ok) {
      throw new Error('Failed to fetch component names');
    }
    return response.json();
  }

  static async getComponentTasks(
    componentName: string,
    startTime?: number,
    endTime?: number
  ): Promise<Task[]> {
    let url = `${API_BASE}/trace?component=${encodeURIComponent(componentName)}`;
    if (startTime !== undefined && endTime !== undefined) {
      url += `&start=${startTime}&end=${endTime}`;
    }
    
    const response = await fetch(url);
    if (!response.ok) {
      throw new Error(`Failed to fetch tasks for component ${componentName}`);
    }
    return response.json();
  }

  static async getComponentData(
    componentName: string,
    startTime?: number,
    endTime?: number
  ): Promise<DataObject[]> {
    let url = `${API_BASE}/data?component=${encodeURIComponent(componentName)}`;
    if (startTime !== undefined && endTime !== undefined) {
      url += `&start=${startTime}&end=${endTime}`;
    }
    
    const response = await fetch(url);
    if (!response.ok) {
      throw new Error(`Failed to fetch data for component ${componentName}`);
    }
    return response.json();
  }

  static async searchTasks(query: string): Promise<Task[]> {
    const response = await fetch(`${API_BASE}/search?q=${encodeURIComponent(query)}`);
    if (!response.ok) {
      throw new Error('Failed to search tasks');
    }
    return response.json();
  }
}