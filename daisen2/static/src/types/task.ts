export interface TaskStep {
  time: number;
  what: string;
  kind: string;
}

export interface Task {
  id: string | number;
  parent_id: string | number;
  kind: string;
  what: string;
  location: string;
  start_time: number;
  end_time: number;
  steps?: TaskStep[] | null;
  yIndex?: number;
  isParentTask?: boolean;
  isMainTask?: boolean;
}

export interface Segment {
  start_time: number;
  end_time: number;
}

export interface SegmentsResponse {
  enabled: boolean;
  segments: Segment[];
}
