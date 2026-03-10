import * as d3 from "d3";
import { isContainerKind } from "./gotypes"
import { UIManager } from "./ui_manager"

export class Monitor {
	uiManager: UIManager
	monitorGroupContainer: HTMLElement
	widgets: Array<Widget>

	constructor(uiManager: UIManager) {
		this.uiManager = uiManager
		this.monitorGroupContainer =
			document.getElementById("monitor-group-container")!
		this.widgets = []
	}

	addWidget(component: string, field: string) {
		const widget = new Widget(component, field, this)

		const widgetDom = widget.createDom()
		this.monitorGroupContainer.appendChild(widgetDom)

		widget.startMonitor()

		if (this.widgets.length == 0) {
			this.uiManager.popUpMonitorGroup()
		}

		this.widgets.push(widget)
		this.uiManager.resize()
	}

	removeWidget(component: string, field: string) {
		const widget = this.getWidget(component, field)
		if (widget == null) {
			return
		}

		widget.stopMonitor()

		const widgetDom = widget.dom!
		widgetDom.remove()

		this.widgets = this.widgets.filter((w) => w != widget)

		if (this.widgets.length == 0) {
			this.uiManager.collapseMonitorGroup()
		}

		this.uiManager.resize()
	}

	getWidget(component: string, field: string): Widget | null {
		for (let i = 0; i < this.widgets.length; i++) {
			const widget = this.widgets[i]
			if (widget.component == component && widget.field == field) {
				return widget
			}
		}

		return null
	}
}

class DataPoint {
	time: number
	value: number

	constructor(time: number, value: number) {
		this.time = time
		this.value = value
	}
}

class Widget {
	component: string
	field: string
	monitor: Monitor
	dom: HTMLElement | null = null
	data: Array<DataPoint>
	interval: number = 0


	constructor(component: string, field: string, monitor: Monitor) {
		this.component = component
		this.field = field
		this.monitor = monitor
		this.data = []
	}

	createDom() {
		const dom = document.createElement("div")
		dom.classList.add("monitor-widget")

		this.dom = dom

		const title = document.createElement("div")
		title.classList.add("title")

		const closeButton = document.createElement("div")
		closeButton.classList.add("close-button")
		closeButton.innerHTML = "&times;"
		closeButton.addEventListener("click", () => {
			this.monitor.removeWidget(this.component, this.field)
		})
		title.appendChild(closeButton)

		const titleText = document.createElement("span")
		titleText.innerHTML = this.component + this.field
		title.appendChild(titleText)

		dom.appendChild(title)

		const chart = document.createElement("div")
		chart.classList.add("chart")
		chart.innerHTML = `<svg>
			<g class="x-axis"></g>
			<g class="y-axis"></g>
			<g class="bar-group"></g>
		</svg>`
		dom.appendChild(chart)

		return dom
	}

	startMonitor() {
		// Register a interval timer to update the chart
		this.interval = setInterval(() => {
			this.updateChart()
		}, 1000)
	}

	stopMonitor() {
		clearInterval(this.interval)
	}

	updateChart() {
		const req = {
			comp_name: this.component,
			field_name: this.field.substring(1),
		}

		Promise.all([
			fetch(`/api/field/${JSON.stringify(req)}`),
			fetch(`/api/now`)
		]).then(([res1, res2]) => {
			return Promise.all([res1.json(), res2.json()])
		}).then(([fieldData, now]) => {
			const newData = fieldData["dict"][fieldData["r"]]
			let data = 0
			if (isContainerKind(newData["k"])) {
				data = newData["l"]
			} else {
				data = newData["v"]
			}

			const dp = new DataPoint(now["now"], data)

			this.data.push(dp)

			if (this.data.length > 300) {
				this.data.shift()
			}

			console.log(data)

			this.render()
		})
	}

	render() {
		const svgDom = this.dom!.querySelector("svg")!
		const svg = d3.select(svgDom)
		const canvasWidth = svgDom.clientWidth
		const canvasHeight = svgDom.clientHeight

		const padding = 8
		const yAxisWidth = 25
		const xAxisHeight = 14
		const contentWidth = canvasWidth - yAxisWidth - padding * 2
		const contentHeight = canvasHeight - xAxisHeight - padding * 2

		const xScale = d3.scaleLinear()
			.domain([
				d3.min(this.data, (d: any) => d.time),
				d3.max(this.data, (d: any) => d.time),
			])
			.range([0, contentWidth])

		const yScale = d3.scaleLinear()
			.domain([0, d3.max(this.data, (d: any) => d.value)])
			.range([contentHeight, 0])

		const xAxis = d3.axisBottom(xScale)
		const yAxis = d3.axisLeft(yScale)

		const xAxisDom = svg.select(".x-axis")
			.attr("transform", `translate(
					${yAxisWidth + padding}, 
					${contentHeight + padding}
				)`)
		const yAxisDom = svg.select(".y-axis")
			.attr("transform", `translate(${yAxisWidth + padding}, ${padding})`)

		xAxisDom.call(xAxis as any)
		yAxisDom.call(yAxis as any)

		const barGroup = svg.select('.bar-group')
		const bars = barGroup.selectAll<SVGRectElement, DataPoint>('rect')
			.data(this.data, (d: DataPoint) => d.time)

		const enterBars = bars.enter().append('rect')
			.attr('class', 'bar')
			.attr('x', (d: any) => xScale(d.time) + padding + yAxisWidth)
			.attr('y', () => padding + contentHeight)
			.attr('width', contentWidth / this.data.length)
			.attr('height', 0)
			.attr('fill', '#666666')

		bars.merge(enterBars)
			.transition()
			.attr('x', (d: any) => xScale(d.time) + padding + yAxisWidth)
			.attr('y', (d: any) => padding + yScale(d.value))
			.attr('width', contentWidth / this.data.length)
			.attr('height', (d: any) => contentHeight - yScale(d.value))
			.attr('fill', '#666666')

		bars.exit().remove()
	}

}