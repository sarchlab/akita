export class Dim {
  x: number;
  y: number;
  width: number;
  height: number;
  startTime: number;
  endTime: number;
}

export class TaskStep {
  time: number;
  what: string;
  kind: string;
}

export class TaskMilestone {
  time: number;
  name: string;
  achieved: boolean;
  color: string;
}

export class Task {
  id: string;
  parent_id: string;
  kind: string;
  what: string;
  location: string;
  milestones: Array<TaskMilestone>;
  start_time: number;
  end_time: number;
  steps: Array<TaskStep>;

  subTasks: Array<Task>;
  level: number;
  dim: Dim;
  isMainTask: boolean;
  isParentTask: boolean;
  yIndex: number;
}
