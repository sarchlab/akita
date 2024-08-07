import * as d3 from 'd3';

interface TreeNode {
  id: string;
  type: string;
  what: string;
  startTime: number;
  endTime: number;
  children?: TreeNode[];
}

function renderReqTree(container: d3.Selection<SVGSVGElement, unknown, null, undefined>, data: TreeNode[]) {
  const margin = { top: 25, right: 90, bottom: 30, left: 50 };
  const containerWidth = container.node().getBoundingClientRect().width;
  const width = containerWidth - margin.left - margin.right;
  const nodeHeight = 60;
  const nodeSpacing = 10;
  const fixedVerticalSpacing = 50;
  function removeDuplicates(nodes: TreeNode[]): TreeNode[] {
    const uniqueNodes: { [key: string]: TreeNode } = {};
    nodes.forEach(node => {
      const key = `${node.type}_${node.id}`;
      if (!uniqueNodes[key] || node.children) {
        uniqueNodes[key] = node;
        if (node.children) {
          node.children = removeDuplicates(node.children);
        }
      }
    });
    return Object.values(uniqueNodes);
  }
  const uniqueData = removeDuplicates(data);
  const height = Math.max(
    (uniqueData.length * (nodeHeight + nodeSpacing)) - margin.top - margin.bottom,
    1000
  );

  container.selectAll("*").remove();

  container.style('max-height', '1000px') 
  .style('overflow-y', 'auto')
  .style('overflow-x', 'hidden');
  
  const svg = container
    .attr("width", width + margin.left + margin.right)
    .attr("height", height + margin.top + margin.bottom)
    .append("g")
    .attr("transform", `translate(${margin.left},${margin.top})`);

  const root = d3.hierarchy({ children: uniqueData } as TreeNode);

  const maxDepth = root.descendants().reduce((max, d) => Math.max(max, d.depth), 0);
  const nodeSpacingY = height / (root.descendants().length / (maxDepth + 1));

  const treeLayout = d3.tree<TreeNode>()
    .size([height, width])
    .separation((a, b) => {
      if (a.parent === b.parent) {
        return 1;
      }
      return 2;
    });


  const treeData = treeLayout(root);

  const depthMap = new Map<number, number>();
  treeData.each(node => {
    if (!depthMap.has(node.depth)) {
      depthMap.set(node.depth, 0);
    }
    node.x = depthMap.get(node.depth);
    depthMap.set(node.depth, node.x + fixedVerticalSpacing);
  });

  const customLinkGenerator = (d: d3.HierarchyPointLink<TreeNode>) => {
    return d3.linkHorizontal()({
      source: [d.source.y, d.source.x],
      target: [d.target.y, d.target.x]
    });
  };

  svg.selectAll(".link")
    .data(treeData.links())
    .enter().append("path")
    .attr("class", "link")
    .attr("d", customLinkGenerator)
    .attr("fill", "none")
    .attr("stroke", "#ccc");

  const node = svg.selectAll(".node")
    .data(treeData.descendants())
    .enter().append("g")
    .attr("class", d => "node" + (d.children ? " node--internal" : " node--leaf"))
    .attr("transform", d => `translate(${d.y},${d.x})`);

  const tooltip = d3.select("body").append("div")
    .attr("class", "tooltip")
    .style("position", "absolute")
    .style("background-color", "white")
    .style("padding", "5px")
    .style("border", "1px solid #ccc")
    .style("border-radius", "4px")
    .style("pointer-events", "none")
    .style("opacity", 0);

  node.append("rect")
    .attr("width", 60)
    .attr("height", 40)
    .attr("x", -50)
    .attr("y", -20)
    .attr("fill", d => d.data.type === "req_in" ? "#4CAF50" : "#2196F3")
    .attr("stroke", "#333")
    .attr("rx", 5)
    .attr("ry", 5)
    .on("mouseover", function(this: SVGRectElement, event: MouseEvent, d: d3.HierarchyPointNode<TreeNode>) {
      tooltip.transition()
        .duration(200)
        .style("opacity", "0.9");
      tooltip.html(`ID: ${d.data.id}<br>What: ${d.data.what}`)
        .style("left", `${event.pageX + 10}px`)
        .style("top", `${event.pageY - 28}px`);
    })
    .on("mouseout", function(this: SVGRectElement) {
      tooltip.transition()
        .duration(500)
        .style("opacity", "0");
    });

  node.append("text")
    .attr("dy", ".35em")
    .attr("x", -20)
    .style("text-anchor", "middle")
    .text(d => d.data.type)
}

export default renderReqTree;