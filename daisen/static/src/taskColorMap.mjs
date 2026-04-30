export function createTaskColorScale(chroma, taskTypeCount) {
  return chroma
    .cubehelix()
    .gamma(0.7)
    .lightness([0.1, 0.7])
    .scale()
    .colors(taskTypeCount + 1);
}

export function buildTaskColorMap(tasks, chroma) {
  const taskTypes = tasks.reduce((types, task) => {
    const kindWhat = task.kind + "-" + task.what;
    types[kindWhat] = true;
    return types;
  }, {});

  const taskTypeArray = Object.keys(taskTypes);
  taskTypeArray.sort();

  const colors = createTaskColorScale(chroma, taskTypeArray.length);
  const colorMap = {};
  taskTypeArray.forEach((taskType, index) => {
    colorMap[taskType] = colors[index + 1];
  });

  return colorMap;
}
