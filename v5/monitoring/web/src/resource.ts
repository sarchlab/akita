import * as d3 from 'd3';

export class ResourceMonitor {
	constructor() {
		// Register interval timer to refresh the resource usage. 
		setInterval(() => {
			this.refreshResourceUsage();
		}, 1000);

		document.getElementById("profile-button")!
			.addEventListener("click", () => {
				this.getProfilingResult();
			})
	}

	refreshResourceUsage() {
		fetch('/api/resource').
			then(res => res.json()).
			then(data => {
				const cpuSpan = document.getElementById('cpu-usage')!;
				const memSpan = document.getElementById('mem-usage')!;

				cpuSpan.innerHTML = this.formatCPUPercent(data.cpu_percent);
				memSpan.innerHTML = this.formatBytes(data.memory_size);
			})
	}

	formatCPUPercent(percent: number) {
		return percent.toFixed(0) + '%';
	}

	formatBytes(bytes: number) {
		const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];

		if (bytes == 0) return '0';

		const i = Math.floor(Math.log(bytes) / Math.log(1024));

		return Math.round(bytes / Math.pow(1024, i)) + ' ' + sizes[i];
	}

	getProfilingResult() {
		fetch('/api/profile').
			then(res => res.json())
			.then(data => {
				console.log(data);
				const network = this.pprofDataToNetwork(data);
				console.log(network);
				this.visualizePProfNetwork(network);
			})
	}

	pprofDataToNetwork(data: any): PProfNetwork {
		const network: PProfNetwork = new PProfNetwork();

		for (const func of data.Function) {
			const node: PProfNode = new PProfNode(func);
			network.nodes.push(node);
		}

		let totalTime = 0;
		for (const sample of data.Sample) {
			totalTime += sample.Value[0];

			for (let i = 0; i < sample.Location.length; i++) {
				const loc = sample.Location[i];
				const node = network.nodes[loc.Line[0].Function.ID - 1];

				node.time += sample.Value[0];
				if (i == 0) {
					node.selfTime += sample.Value[0];
				}
			}

			for (let i = 0; i < sample.Location.length - 1; i++) {
				const caller = sample.Location[i + 1].Line[0].Function.ID - 1;
				const callee = sample.Location[i].Line[0].Function.ID - 1;

				const edge = network.getOrCreateEdge(caller, callee);
				edge.time += sample.Value[0];
			}
		}

		for (const node of network.nodes) {
			node.selfTimePercentage = node.selfTime / totalTime;
			node.timePercentage = node.time / totalTime;
		}

		network.nodes = network.nodes.sort((a, b) => b.time - a.time);

		for (const edge of network.edges) {
			edge[1].timePercentage = edge[1].time / totalTime;
		}

		for (let i = 0; i < network.nodes.length; i++) {
			const node = network.nodes[i];
			node.index = i;
		}

		return network;
	}

	visualizePProfNetwork(network: PProfNetwork) {
		const rightPane = document.getElementById('right-pane')!;
		rightPane.innerHTML = '<div id="pprof-tooltip"><div>';

		const svg = document.createElementNS(
			'http://www.w3.org/2000/svg', 'svg')!;
		rightPane.appendChild(svg);

		const d3SVG = d3.select<SVGElement, unknown>(svg);
		d3SVG.attr('width', '100%');
		d3SVG.attr('height', '100%');
		d3SVG.attr('overflow', 'scroll');

		d3SVG.append("defs").append("marker")
			.attr("id", "arrowhead")
			.attr("viewBox", "0 -66 200 200")
			.attr("refX", 5)
			.attr("refY", 2)
			.attr("markerWidth", 18)
			.attr("markerHeight", 12)
			.attr("orient", "auto")
			.attr("markerUnits", "userSpaceOnUse")
			.append("path")
			.attr("d", "M0,-66 L0,66 L200,0");


		const nodeGroup = d3SVG.append('g').attr('id', 'pprof-nodes');
		const edgeGroup = d3SVG.append('g').attr('id', 'pprof-edges');
		d3SVG.call(
			d3.zoom<SVGElement, unknown>().
				on('zoom', (e) => {
					nodeGroup.attr('transform', e.transform);
					edgeGroup.attr('transform', e.transform);
				}));

		const colorScale = d3.scaleSequential(
			d3.interpolateOranges).domain([0, 1]);


		// Create the nodes.
		const node = nodeGroup.selectAll<SVGRectElement, PProfNode>('rect')
			.data(network.nodes, (d: PProfNode) => d.func.ID)
			.enter().append('g')
			.attr('class', 'pprof-node')
			.attr('transform', (_: PProfNode, i) => {
				return `translate(0, ${i * 30})`;
			});

		node.append('rect')
			.attr('width', '15')
			.attr('height', '15')
			.attr('fill', (d: PProfNode) => {
				return colorScale(d.timePercentage);
			})
			.attr('stroke', '#000')
			.attr('stroke-width', '1px')
			.attr('rx', '5px')
			.attr('ry', '5px')
			.attr('x', '2')
			.attr('y', '2')
			.attr('class', 'pprof-node-rect')
			.on("mouseover", (e: MouseEvent, d: PProfNode) => {
				this.showTooltip(e, d, network)
			}).on("mouseout", () => {
				this.hideTooltip()
			});

		node.append('rect')
			.attr('width', '15')
			.attr('height', '15')
			.attr('fill', (d: PProfNode) => {
				return colorScale(d.selfTimePercentage);
			})
			.attr('stroke', '#000')
			.attr('stroke-width', '1px')
			.attr('rx', '5px')
			.attr('ry', '5px')
			.attr('x', '20')
			.attr('y', '2')
			.attr('class', 'pprof-node-rect')
			.on("mouseover", (e: MouseEvent, d: PProfNode) => {
				this.showTooltip(e, d, network)
			}).on("mouseout", () => {
				this.hideTooltip()
			});

		node.append('text')
			.attr('x', '45')
			.attr('y', '15')
			.attr('class', 'pprof-node-text')
			.text((d: PProfNode) => {
				const funcName = d.func['Name'];
				const funcNameParts = funcName.split('/');
				return funcNameParts[funcNameParts.length - 1];
			})

		// Add edges
		edgeGroup.selectAll<SVGPathElement, PProfEdge>('path')
			.data(network.edges.values())
			.enter().append('path')
			.attr('d', (d: PProfEdge) => {
				const x1 = 0;
				const y1 = d.caller.index * 30 + 7;
				const x2 = -1;
				const y2 = d.callee.index * 30 + 7;

				const r = Math.abs(y2 - y1) / 2;
				// Return a path that represent a semi-circle between the two nodes.

				let direction = '0';
				// if (y2 < y1) {
				// 	direction = '1';
				// }

				return `M ${x1} ${y1} A ${r} ${r} 0 0 ${direction} ${x2} ${y2}`;
			}).attr('stroke', (_: PProfEdge) => {
				// return colorScale(d.timePercentage);
				return '#999999';
			}).attr('stroke-width', (d: PProfEdge) => {
				return d.timePercentage * 10;
			}).attr('fill', 'none')
			.attr("marker-end", "url(#arrowhead)")
			.attr('marker-end-size', '6');
	}

	showTooltip(e: MouseEvent, d: PProfNode, network: PProfNetwork) {
		const tooltip = document.getElementById('pprof-tooltip')!;
		tooltip.innerHTML = `
			<div>${d.func['Name']}</div>
			<div>Time: ${d.timePercentage * 100}%</div>
			<div>Self time: ${d.selfTimePercentage * 100}%</div>
		`;
		tooltip.style.display = 'block';
		tooltip.style.left = `${e.pageX}px`;
		tooltip.style.top = `${e.pageY}px`;

		const edgeGroup = d3.select('#pprof-edges');
		edgeGroup.selectAll('path')
			.data(network.edges.values())
			.filter((e: PProfEdge) => {
				return e.caller.index !== d.index && e.callee.index !== d.index;
			}).attr('opacity', '.3');
	}

	hideTooltip() {
		const tooltip = document.getElementById('pprof-tooltip')!;
		tooltip.style.display = 'none';

		const edgeGroup = d3.select('#pprof-edges');
		edgeGroup.selectAll('path').attr('opacity', '1');
	}
}



class PProfNetwork {
	nodes: Array<PProfNode>;
	edges: Map<string, PProfEdge>;

	constructor() {
		this.nodes = [];
		this.edges = new Map<string, PProfEdge>();
	}

	static getEdgeName(callerIndex: number, calleeIndex: number) {
		return `${callerIndex}-${calleeIndex}`;
	}

	getOrCreateEdge(callerIndex: number, calleeIndex: number) {
		const edgeName = PProfNetwork.getEdgeName(callerIndex, calleeIndex);
		let edge = this.edges.get(edgeName);

		if (edge == null) {
			edge = new PProfEdge(
				this.nodes[callerIndex],
				this.nodes[calleeIndex]
			);
			this.edges.set(edgeName, edge);

			this.nodes[callerIndex].addEdge(edge);
		}

		return edge;
	}
}

class PProfNode {
	index: number = 0;
	func: any;
	selfTime: number;
	time: number
	edges: Array<PProfEdge>;
	timePercentage: number = 0;
	selfTimePercentage: number = 0;

	constructor(func: any) {
		this.func = func;
		this.selfTime = 0;
		this.time = 0;
		this.edges = [];
	}

	addEdge(edge: PProfEdge) {
		this.edges.push(edge);
	}
}

class PProfEdge {
	time: number;
	timePercentage: number = 0;
	caller: PProfNode;
	callee: PProfNode;

	constructor(caller: PProfNode, callee: PProfNode) {
		this.time = 0;
		this.caller = caller;
		this.callee = callee;
	}
}

