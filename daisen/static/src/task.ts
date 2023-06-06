export class Dim {
  x: number;
  y: number;
  width: number;
  height: number;
  startTime: number;
  endTime: number;
}

export class Task {
  id: string;
  parent_id: string;
  kind: string;
  what: string;
  where: string;
  start_time: number;
  end_time: number;

  subTasks: Array<Task>;
  level: number;
  dim: Dim;
  isMainTask: boolean;
  isParentTask: boolean;
  yIndex: number;
}
