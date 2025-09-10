class TaskYIndexAssigner {
    /**
     * Assign the y index of each task.
     * @param {Task[]} tasks
     * @return {number} maxY
     */
    assign(tasks) {
        return this._assignYIndex(tasks);
    }

    _assignYIndex(tasks) {
        let assignment = [];
        let maxYIndex = 0

        tasks.sort((a, b) => (a.start_time - b.start_time));

        tasks.forEach(t => {
            let index = 0;
            while (true) {
                if (!this._hasConflictWithAssignment(t, assignment[index])) {
                    break;
                }
                index++;
            }
            this._assignYIndexToOneTask(t, index, assignment);

            if (index > maxYIndex) {
                maxYIndex = index;
            }
        })

        return maxYIndex;
    }

    _hasConflictWithAssignment(task, assigmnentInOneRow) {
        if (assigmnentInOneRow === undefined) {
            return false
        }

        for (let i = 0; i < assigmnentInOneRow.length; i++) {
            let t = assigmnentInOneRow[i];
            if (t.start_time <= task.start_time &&
                t.end_time > task.start_time) {
                return true;
            }

            if (t.start_time < task.end_time &&
                t.end_time >= task.end_time) {
                return true;
            }

            if (task.start_time <= t.start_time &&
                task.end_time >= t.end_time) {
                return true;
            }

            if (task.start_time >= t.start_time &&
                task.end_time <= t.end_time) {
                return true
            }
        }
        return false
    }

    /**
     * @param {Array.{Object}} task
     * @param {number} index
     * @param {Array.{Array.Object}} assignment
     */
    _assignYIndexToOneTask(task, index, assignment) {
        if (assignment.length < index) {
            throw "Something's wrong :)";
        }

        if (assignment.length === index) {
            assignment.push([]);
        }

        assignment[index].push(task);

        task.yIndex = index;
    }
}

export default TaskYIndexAssigner;