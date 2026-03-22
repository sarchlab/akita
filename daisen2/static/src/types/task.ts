/** Matches the shape returned by /api/trace */
export interface TaskStep {
  time: number;
  what: string;
  kind: string;
}

export interface TaskMilestone {
  time: number;
  name: string;
  achieved: boolean;
  color: string;
}

export interface Task {
  id: string;
  parent_id: string;
  kind: string;
  what: string;
  location: string;
  milestones: TaskMilestone[];
  start_time: number;
  end_time: number;
  steps: TaskStep[];

  /* Client-side augmentation */
  subTasks?: Task[];
  level?: number;
  isMainTask?: boolean;
  isParentTask?: boolean;
  yIndex?: number;
}

/** Matches /api/segments response */
export interface Segment {
  start_time: number;
  end_time: number;
}

export interface SegmentsResponse {
  enabled: boolean;
  segments: Segment[];
}
