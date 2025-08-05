import * as chroma from "chroma-js";
export class TaskColorCoder {
    constructor() { }
    recode(tasks) {
        this._colorMap = {};
        let taskTypes = {};
        taskTypes = tasks.reduce((types, task) => {
            let kindWhat = task.kind + "-" + task.what;
            if (!(kindWhat in taskTypes)) {
                taskTypes[kindWhat] = true;
            }
            return taskTypes;
        }, taskTypes);
        let taskTypeArray = Object.keys(taskTypes);
        taskTypeArray.sort();
        const colors = chroma
            .cubehelix()
            .gamma(0.7)
            .lightness([0.1, 0.7])
            .scale()
            .colors(taskTypeArray.length + 1);
        taskTypeArray.forEach((t, i) => {
            this._colorMap[t] = colors[i + 1];
        });
    }
    lookup(task) {
        let kindWhat = task.kind + "-" + task.what;
        return this.lookupWithText(kindWhat);
    }
    lookupWithText(text) {
        if (text in this._colorMap) {
            return this._colorMap[text];
        }
        throw text + " is not in color map";
    }
    get colorMap() {
        return this._colorMap;
    }
}
export default TaskColorCoder;
