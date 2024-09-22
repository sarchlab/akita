import * as d3 from 'd3';
import { TaskPage } from './taskpage';
import { Task } from './task'; 
interface TreeNode {
  id: string;
  type: string;
  where: string;
  startTime?: number;
  endTime?: number;
  left?: TreeNode[];
  right?: TreeNode[];
}

function renderReqTree(container: d3.Selection<HTMLElement, unknown, null, undefined>, data: TreeNode, taskPage: TaskPage) {
  container.selectAll("*").remove();

  const containerDiv = container.append('div')
    .style('width', '100%')
    .style('overflow-x', 'auto')
    .style('overflow-y', 'hidden');

  const svg = containerDiv.append('svg')
    .attr('height', '300px');  

  renderTree(svg, data, taskPage);
}

function renderTree(container: d3.Selection<SVGSVGElement, unknown, null, undefined>, data: TreeNode, taskPage: TaskPage) {
  const margin = { top: -50, right: 20, bottom: 20, left: 0 };
  // const width = container.node().getBoundingClientRect().width - margin.left - margin.right;
  const height = 260; 
  const nodeWidth = 140;
  const gapWidth = -100;
  const totalNodes = (data.left?.length || 0) + 1 + (data.right?.length || 0);
  const requiredWidth = totalNodes * nodeWidth + (totalNodes - 1) * gapWidth;
  const containerWidth = container.node().parentElement.getBoundingClientRect().width;
  const width = Math.max(containerWidth, requiredWidth);

  container.attr('width', width);
  const svg = container.append("g")
    .attr("transform", `translate(${margin.left},${margin.top})`);

  const centerX = containerWidth / 2;
  const yStep = height / 3;

  const calculateXPositions = (nodes: TreeNode[], totalWidth: number) => {
    const nodeWidth = 150;
    const totalNodesWidth = nodes.length * nodeWidth;
    const remainingSpace = Math.max(totalWidth - totalNodesWidth, 0);
    const gap = nodes.length > 1 ? remainingSpace / (nodes.length - 1) : 0;
    const startX = (totalWidth - (totalNodesWidth + (nodes.length - 1) * gap)) / 2;
    return nodes.map((_, index) => startX + (index * (nodeWidth + gap)) + (nodeWidth / 2));
  };
  const deduplicateNodes = (nodes: TreeNode[]): TreeNode[] => {
    const uniqueNodes = new Map<string, TreeNode>();
    nodes.forEach(node => {
      if (!uniqueNodes.has(node.where)) {
        uniqueNodes.set(node.where, node);
      }
    });
    return Array.from(uniqueNodes.values());
  };

  const leftNodes = data.left ? deduplicateNodes(data.left) : [];
  const rightNodes = data.right ? deduplicateNodes(data.right) : [];

  if (leftNodes.length > 0) {
    const xPositions = calculateXPositions(leftNodes, containerWidth);
    leftNodes.forEach((node, index) => {
      renderNode(svg, node, xPositions[index], yStep, 'left', taskPage);
      drawConnector(svg, xPositions[index], yStep + 15, centerX, 2 * yStep - 15);
    });
  }

  renderNode(svg, data, centerX, 2 * yStep, 'center', taskPage);
  
  if (rightNodes.length > 0) {
    const xPositions = calculateXPositions(rightNodes, containerWidth);
    rightNodes.forEach((node, index) => {
      renderNode(svg, node, xPositions[index], 3 * yStep, 'right', taskPage);
      drawConnector(svg, centerX, 2 * yStep + 15, xPositions[index], 3 * yStep - 15);
    });
  }
}

function drawConnector(svg: d3.Selection<SVGGElement, unknown, null, undefined>, x1: number, y1: number, x2: number, y2: number) {
  const midY = (y1 + y2) / 2;
  svg.append("path")
    .attr("d", `M${x1},${y1} L${x1},${midY} L${x2},${midY} L${x2},${y2}`)
    .attr("fill", "none")
    .attr("stroke", "#ccc");
}

function renderNode(svg: d3.Selection<SVGGElement, unknown, null, undefined>, node: TreeNode, x: number, y: number, position: 'left' | 'right' | 'center', taskPage: TaskPage) {
  const g = svg.append("g")
    .attr("transform", `translate(${x},${y})`);

  g.append("rect")
    .attr("width", 150)
    .attr("height", 30)
    .attr("x", -75)
    .attr("y", -15)
    .attr("fill", getNodeColor(node, position))
    .attr("stroke", "#333")
    .attr("rx", 4)
    .attr("ry", 4);

  g.append("text")
    .attr("dy", ".35em")
    .attr("text-anchor", "middle")
    .style("font-size", "10px")
    .style("font-weight", "bold")
    .style("fill", "#132C33")
    .text(node.where)
    .each(function(this: SVGTextElement) {
      const self = d3.select(this);
      let textLength = this.getComputedTextLength();
      let text = self.text();
      while (textLength > 130 && text.length > 0) {
        text = text.slice(0, -1);
        self.text(text + '...');
        textLength = this.getComputedTextLength();
      }
    });

  const tooltip = d3.select("body").append("div")
    .attr("class", "tooltip")
    .style("background-color", "white")
    .style("opacity", 0);

  g.on("mouseover", (event: MouseEvent) => {
    tooltip.transition()
      .duration(200)
      .style("opacity", 0.9);
    tooltip.html(`Type: ${node.type}<br/>Where: ${node.where}`)
      .style("left", (event.pageX + 10) + "px")
      .style("top", (event.pageY - 28) + "px");
    const componentName = node.where;
    const tasksToHighlight = taskPage.getTasksByComponent(componentName); 
    taskPage.highlight((t: Task) => tasksToHighlight.some(ht => ht.id === t.id));
  })
  .on("mouseout", () => {
    tooltip.transition()
      .duration(500)
      .style("opacity", 0);
    taskPage.highlight(null);
  });
}

function getNodeColor(node: TreeNode, position: 'left' | 'right' | 'center'): string {
  switch (position) {
    case 'center':
      return "#D1285B";  
    case 'left':
      return "#FCCF65"; 
    case 'right':
      return "#098761"; 
    default:
      return "#cccccc"; 
  }
}

export default renderReqTree;