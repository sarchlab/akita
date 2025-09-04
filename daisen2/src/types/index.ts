// Core data types used throughout the application

export interface Task {
  id: string;
  parent_id: string;
  name: string;
  what: string;
  where: string;
  start_time: number;
  end_time: number;
  level: number;
  dim: Dim;
}

export interface Dim {
  x: number;
  y: number;
  width: number;
  height: number;
  startTime: number;
  endTime: number;
}

export interface TimeValue {
  time: number;
  value: number;
}

export interface SimulationInfo {
  id: string;
  name: string;
  start_time: number;
  end_time: number;
}

export interface ComponentInfo {
  name: string;
  tasks: Task[];
  widgets?: DataObject[];
}

export interface DataObject {
  info_type: string;
  data: TimeValue[];
}

export interface YAxisOption {
  optionValue: string;
  html: string;
}

export interface ChatMessage {
  role: 'user' | 'assistant' | 'system';
  content: string;
}

export interface ZoomHandler {
  domElement(): HTMLElement;
  setTimeRange(startTime: number, endTime: number, reloadData: boolean): void;
  getStartTime(): number;
  getEndTime(): number;
  reloadData(): void;
}