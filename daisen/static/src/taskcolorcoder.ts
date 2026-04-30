import chroma from "chroma-js";
import { Task } from "./task";
import { buildTaskColorMap } from "./taskColorMap.mjs";

export class TaskColorCoder {
  private _colorMap: Object;

  constructor() { }

  recode(tasks: Array<Task>) {
    this._colorMap = buildTaskColorMap(tasks, chroma);
  }

  lookup(task: Task): string {
    let kindWhat = task.kind + "-" + task.what;
    return this.lookupWithText(kindWhat);
  }

  lookupWithText(text: string): string {
    if (text in this._colorMap) {
      return this._colorMap[text];
    }
    throw text + " is not in color map";
  }

  get colorMap(): Object {
    return this._colorMap;
  }
}

export default TaskColorCoder;
