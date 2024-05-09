import { VarKind, isContainerKind, isDirectKind } from "./gotypes"
import { Monitor } from "./monitor"
import * as d3 from "d3"
import * as sankey from "d3-sankey"

export class ComponentDetailView {
    name: string
    monitor: Monitor
    running: Boolean = false
    intervalHandle: number = 0
    measurementMode: string = "Byte"
    accumulationMode: string = "Last"
    sankeyCumulative: any = {
        epochs: new Set<Number>(),
        linkValues: new Map<string, Number>(), // Source|Dest|Type
    }

    constructor(name: string, monitor: Monitor) {
        this.name = name
        this.monitor = monitor
    }

    populate() {
        fetch(`/api/component/${this.name}`)
            .then(res => res.json())
            .then((res: any) => {
                const container = document.getElementById('central-pane')
                const object = res["dict"][res["r"]]

                this.showComponent(object, res["dict"], container!)

                const sankeyContainer = document.getElementById('right-pane')
                this.showSankey(sankeyContainer!)
            })
    }

    showComponent(comp: any, dict: any, container: HTMLElement) {
        container.innerHTML = ""

        const componentContainer = document.createElement("div");
        componentContainer.classList.add("component-container")
        container.appendChild(componentContainer)

        const componentHeader = document.createElement("div")
        componentHeader.classList.add("component-header")
        componentHeader.innerHTML = `
            <div class="component-name">${this.name}</div>
        `

        const headerForm = document.createElement("form")
        headerForm.classList.add('form-inline', 'my-2', 'my-lg-0')
        componentHeader.appendChild(headerForm)

        const tickBtn = document.createElement("button")
        tickBtn.classList.add("btn", "btn-success", "float-right")
        tickBtn.innerHTML = "Tick"
        headerForm.appendChild(tickBtn)

        tickBtn.addEventListener('click', (e: Event) => {
            e.preventDefault()
            e.stopImmediatePropagation()
            e.stopPropagation()

            fetch(`/api/tick/${this.name}`)
        })


        componentContainer.appendChild(componentHeader)

        const componentDetailContainer = document.createElement("div")
        componentDetailContainer.classList.add('component-detail-container')
        componentContainer.appendChild(componentDetailContainer)

        if (!dict) {
            dict = []
        }

        this.showContent(comp, dict, "", componentDetailContainer, "")
    }

    showSankey(container: HTMLElement) {
        let hangAnalyzerBtn = document.querySelector<HTMLElement>('.auto-refresh-btn')
        if (hangAnalyzerBtn !== null && hangAnalyzerBtn.classList.contains('btn-primary')) {
            hangAnalyzerBtn.click()
        }

        container.innerHTML = ""

        const toolbar = document.createElement("div")
		toolbar.classList.add("btn-toolbar", "mb-3")
		container.appendChild(toolbar)

        const sankeyContainer = document.createElement("div");
        sankeyContainer.classList.add("sankey-container")
        container.appendChild(sankeyContainer)

        const canvas = document.createElementNS("http://www.w3.org/2000/svg", 'svg')
        canvas.classList.add("sankey-diagram")
        sankeyContainer.appendChild(canvas)

        canvas.setAttribute('width', '100%')
        canvas.setAttribute('height', '100%')

		const groupType = document.createElement("div")
		groupType.classList.add("input-group")
		groupType.setAttribute("role", "group")
		groupType.innerHTML = `<div class="input-group-text">Count By:</div>`
		toolbar.appendChild(groupType)

        const dataVolumeBtn = document.createElement("button")
		dataVolumeBtn.classList.add("btn", "btn-primary")
		dataVolumeBtn.innerHTML = "Bytes"
		groupType.appendChild(dataVolumeBtn)

		const msgVolumeBtn = document.createElement("button")
		msgVolumeBtn.classList.add("btn", "btn-outline-primary")
		msgVolumeBtn.innerHTML = "Messages"
        groupType.appendChild(msgVolumeBtn)

        dataVolumeBtn.addEventListener("click", (_: Event) => {
			this.measurementMode = "Byte"

			this.getDataAndDrawSankey(canvas)

			dataVolumeBtn.classList.remove('btn-outline-primary')
			dataVolumeBtn.classList.add('btn-primary')
			msgVolumeBtn.classList.remove('btn-primary')
			msgVolumeBtn.classList.add('btn-outline-primary')

		})

		msgVolumeBtn.addEventListener("click", (_: Event) => {
			this.measurementMode = "Msg"

			this.getDataAndDrawSankey(canvas)

			msgVolumeBtn.classList.remove('btn-outline-primary')
			msgVolumeBtn.classList.add('btn-primary')
			dataVolumeBtn.classList.remove('btn-primary')
			dataVolumeBtn.classList.add('btn-outline-primary')
		})

        const epochType = document.createElement("div")
		epochType.classList.add("input-group")
		epochType.setAttribute("role", "group")
		epochType.innerHTML = `<div class="input-group-text">Show Data From:</div>`
		toolbar.appendChild(epochType)

        const lastEpochDataBtn = document.createElement("button")
		lastEpochDataBtn.classList.add("btn", "btn-primary")
		lastEpochDataBtn.innerHTML = "Last Epoch"
		epochType.appendChild(lastEpochDataBtn)

		const cumulativeDataBtn = document.createElement("button")
		cumulativeDataBtn.classList.add("btn", "btn-outline-primary")
		cumulativeDataBtn.innerHTML = "All"
        epochType.appendChild(cumulativeDataBtn)

        lastEpochDataBtn.addEventListener("click", (_: Event) => {
			this.accumulationMode = "Last"

			this.getDataAndDrawSankey(canvas)

			lastEpochDataBtn.classList.remove('btn-outline-primary')
			lastEpochDataBtn.classList.add('btn-primary')
			cumulativeDataBtn.classList.remove('btn-primary')
			cumulativeDataBtn.classList.add('btn-outline-primary')

		})

		cumulativeDataBtn.addEventListener("click", (_: Event) => {
			this.accumulationMode = "Cumulative"

			this.getDataAndDrawSankey(canvas)

			cumulativeDataBtn.classList.remove('btn-outline-primary')
			cumulativeDataBtn.classList.add('btn-primary')
			lastEpochDataBtn.classList.remove('btn-primary')
			lastEpochDataBtn.classList.add('btn-outline-primary')
		})

        const autoRefreshBtn = document.createElement("button")
		autoRefreshBtn.classList.add("btn", "btn-outline-primary", "ms-3", "auto-sankey-refresh-btn")
		autoRefreshBtn.innerHTML = "Stop Refresh"
		toolbar.appendChild(autoRefreshBtn)

		autoRefreshBtn.addEventListener("click", (_: Event) => {
			if (this.running) {
				autoRefreshBtn.classList.remove('btn-primary')
				autoRefreshBtn.classList.add('btn-outline-primary')
				autoRefreshBtn.innerHTML = "Auto Refresh"

                this.stopAnalyzing()
			} else {
				autoRefreshBtn.classList.remove('btn-outline-primary')
				autoRefreshBtn.classList.add('btn-primary')
				autoRefreshBtn.innerHTML = "Stop Refresh"

                this.startAnalyzing(canvas)
			}
		})

        this.startAnalyzing(canvas)
    }

    getDataAndDrawSankey(canvas: SVGSVGElement) {
        fetch(`/api/traffic/${this.name}`)
        .then(res => res.json())
        .then((res: any) => {
            this.drawSankey(res, canvas)
        })
    }

    startAnalyzing(canvas: SVGSVGElement) {
		this.getDataAndDrawSankey(canvas)

		if (!this.running) {
			this.intervalHandle = window.setInterval(() => {
				this.getDataAndDrawSankey(canvas)
			}, 2000)
			this.running = true
		}
	}

	stopAnalyzing() {
		if (this.running) {
			window.clearInterval(this.intervalHandle)
			this.running = false
		}
	}

    showContent(
        s: any, dict: any,
        keyChain: string,
        container: HTMLElement, fieldPrefix: string,
    ) {
        container.innerHTML = ""

        switch (s['k']) {
            case 1:
            case 2:
            case 3:
            case 4:
            case 5:
            case 6:
            case 7:
            case 8:
            case 9:
            case 10:
            case 11:
            case 13:
            case 14:
            case 15:
            case 24:
                this.showDirectValue(s['v'], container)
                break;
            case VarKind.Map:
                this.showMap(s['v'], dict, keyChain, container, fieldPrefix)
                break;
            case VarKind.Slice:
                this.showSlice(s['v'], dict, keyChain, container, fieldPrefix)
                break;
            case VarKind.Struct:
                this.showStruct(s['v'], dict, keyChain, container, fieldPrefix)
                break;
            default:
                console.error(`value kind ${s['k']} is not supported`)
        }
    }

    showDirectValue(s: any, container: HTMLElement) {
        container.innerHTML = s
    }

    showMap(
        s: Array<any>,
        dict: any,
        keyChain: string,
        container: HTMLElement,
        fieldPrefix: string,
    ) {
        const table = document.createElement('table');
        table.classList.add('detail-table')
        container.appendChild(table)

        for (let i = 0; i < s.length; i++) {
            this.showValue(dict, s, `${i}`, keyChain, fieldPrefix, table)
        }
    }


    showSlice(
        s: any,
        dict: any,
        keyChain: string,
        container: HTMLElement,
        fieldPrefix: string,
    ) {
        const table = document.createElement('table');
        table.classList.add('detail-table')
        container.appendChild(table)

        let fields = Object.keys(s);
        fields = fields.sort()

        fields.forEach(f => {
            this.showValue(dict, s, f, keyChain, fieldPrefix, table)
        });
    }

    showStruct(s: any, dict: any, keyChain: string,
        container: HTMLElement,
        fieldPrefix: string,
    ) {
        const table = document.createElement('table');
        table.classList.add('detail-table')
        container.appendChild(table)

        let fields = Object.keys(s);
        fields = fields.sort()

        fields.forEach(f => {
            this.showValue(dict, s, f, keyChain, fieldPrefix, table)
        });
    }

    private showValue(
        dict: any, object: any, key: string,
        keyChain: string,
        fieldPrefix: string,
        table: HTMLTableElement,
    ) {
        const row = document.createElement('tr')
        const cell = document.createElement('td')

        this.showValueTitle(dict, object, key, keyChain, cell)
        this.shouldValueSubContent(dict, object, key, keyChain,
            cell, fieldPrefix)

        row.appendChild(cell)
        table.appendChild(row)
    }

    private shouldValueSubContent(
        dict: any,
        object: any,
        key: string,
        keyChain: string,
        cell: HTMLTableCellElement,
        fieldPrefix: string,
    ) {
        const kind = dict[object[key]]['k']
        keyChain += `.${key}`

        if (!isDirectKind(kind)) {
            const valueContainer = document.createElement('div')
            valueContainer.classList.add("field-sub-container")
            valueContainer.classList.add('collapsed')
            cell.appendChild(valueContainer)

            cell.addEventListener('click', (e: Event) => {
                e.stopPropagation()

                if (valueContainer.classList.contains('collapsed')) {
                    this.showField(`${fieldPrefix}${key}`,
                        keyChain,
                        valueContainer)
                    valueContainer.classList.remove('collapsed')
                    valueContainer.classList.add('expanded')

                    cell.getElementsByClassName('field-title-chevron-right')[0].
                        classList.add('hidden')
                    cell.getElementsByClassName('field-title-chevron-down')[0].
                        classList.remove('hidden')
                } else {
                    valueContainer.classList.remove('expanded')
                    valueContainer.classList.add('collapsed')
                    cell.getElementsByClassName('field-title-chevron-right')[0].
                        classList.remove('hidden')
                    cell.getElementsByClassName('field-title-chevron-down')[0].
                        classList.add('hidden')
                }
            })
        } else {
            cell.addEventListener('click', (_: Event) => {
                // e.stopPropagation()
            })
        }
    }

    private showValueTitle(
        dict: any,
        object: any,
        key: string,
        keyChain: string,
        cell: HTMLTableCellElement,
    ) {
        const kind = dict[object[key]]['k']
        const type = dict[object[key]]['t']
        const value = dict[object[key]]['v']
        const length = dict[object[key]]['l']

        keyChain += `.${key}`

        const fieldTitle = document.createElement('div')
        fieldTitle.classList.add('field-title')

        const flagButton = document.createElement('div')
        flagButton.classList.add('flag-button')
        flagButton.innerHTML = `
            <span class="field-title-flag-regular">
                <i class="fa-regular fa-flag"></i>
            </span>
            <span class="field-title-flag-solid hidden">
                <i class="fa-solid fa-flag"></i>
            </span>`

        if (isDirectKind(kind)) {
            fieldTitle.innerHTML +=
                `<span class="field-title-circle">
                    -
                </span>`
        } else {
            fieldTitle.innerHTML = `
                <span class="field-title-chevron-right">
                    <i class="fa-solid fa-chevron-right fa-xs"></i>
                </span>
                <span class="field-title-chevron-down hidden">
                    <i class="fa-solid fa-chevron-down fa-xs"></i>
                </span>
                `
        }

        if (this.isMonitorableKind(kind)) {
            fieldTitle.appendChild(flagButton)
        }

        fieldTitle.innerHTML += `
            <span class="field-name">${key}</span>
            <span class="field-type">${type}</span>`

        if (isDirectKind(kind)) {
            fieldTitle.classList.add('field-title-non-expandable')
            fieldTitle.innerHTML += `
                <span class="field-value">${value}</span>
            `
        } else {
            fieldTitle.classList.add('field-title-expandable')
        }

        if (isContainerKind(kind)) {
            fieldTitle.innerHTML += `
                <span class="field-value">${length}</span>
            `
        }

        if (this.isMonitorableKind(kind)) {
            fieldTitle.querySelector('.flag-button')!
                .addEventListener('click', (e: Event) => {
                    e.stopPropagation()
                    const selected = this.toggleFlag(fieldTitle)

                    if (selected) {
                        this.monitor.addWidget(this.name, keyChain)
                    } else {
                        this.monitor.removeWidget(this.name, keyChain)
                    }
                })
        }

        cell.appendChild(fieldTitle)
    }

    toggleFlag(button: HTMLElement): boolean {
        const regularFlag = button.querySelector('.field-title-flag-regular')
        const solidFlag = button.querySelector('.field-title-flag-solid')

        if (regularFlag!.classList.contains('hidden')) {
            regularFlag!.classList.remove('hidden')
            solidFlag!.classList.add('hidden')
            return false
        } else {
            regularFlag!.classList.add('hidden')
            solidFlag!.classList.remove('hidden')
            return true
        }
    }

    isMonitorableKind(kind: number) {
        const list = [
            VarKind.Int,
            VarKind.Int8,
            VarKind.Int16,
            VarKind.Int32,
            VarKind.Int64,
            VarKind.Uint,
            VarKind.Uint8,
            VarKind.Uint16,
            VarKind.Uint32,
            VarKind.Uint64,
            VarKind.Float32,
            VarKind.Float64,
            VarKind.Chan,
            VarKind.Map,
            VarKind.Slice,
        ]
        return list.includes(kind)
    }

    showField(field: string, keyChain: string, container: HTMLElement) {
        const req = {
            comp_name: this.name,
            field_name: field,
        }
        fetch(`/api/field/${JSON.stringify(req)}`)
            .then(res => res.json())
            .then(res => {
                this.showContent(
                    res["dict"][res["r"]],
                    res["dict"],
                    keyChain,
                    container,
                    `${field}.`)
            })
    }

    drawSankey(data: any, target: SVGSVGElement) {
        const canvas = d3.select<SVGSVGElement, unknown>(target)

        canvas.selectAll("*").remove()

        const canvasDims = target.getBoundingClientRect()
        const canvasHeight = canvasDims.height
        const canvasWidth = canvasDims.width

        // Originate: This component is the source of the data
        // Terminate: This component is the destination of the data

        const sankOriginates = sankey.sankey()
        .nodeWidth(10)
        .nodePadding(20)
        .extent([[canvasWidth * 2 / 3, canvasHeight / 3], [canvasWidth, canvasHeight * 2 / 3]])
        .nodeId((d: any) => d.name)

        const sankTermiates = sankey.sankey()
        .nodeWidth(10)
        .nodePadding(20)
        .extent([[0, canvasHeight / 3], [canvasWidth / 3, canvasHeight * 2 / 3]])
        .nodeId((d: any) => d.name)

        this.sankeyCumulative.epochs.add(data.find((entry: any) => entry.value > 0).start)

        let linksRaw = data.map((entry: any) => {return {"source": entry.localPort, "target": entry.remotePort, "value": entry.value, "type": entry.unit}})
        // There's a Map.groupBy function, but it doesn't work in safari
        let linksMap = this.groupByLinks(linksRaw)
        let links: any[] = []
        for (let entry of linksMap.entries()) {
            let parsed = entry[0].split('|')
            let value = entry[1]
            if (this.sankeyCumulative.linkValues.has(entry[0])) {
                let toStore = this.sankeyCumulative.linkValues.get(entry[0]) + value
                if (this.accumulationMode === "Cumulative") {
                    value = toStore
                }
                this.sankeyCumulative.linkValues.set(entry[0], toStore)
            } else {
                this.sankeyCumulative.linkValues.set(entry[0], value)
            }

            links.push({"source": parsed[0], "target": parsed[1], "value": value, "names": [parsed[0], parsed[1]], "type": parsed[2], "fullName": entry[0]})
        }
        if (links.length < this.sankeyCumulative.linkValues.size) {
            this.sankeyCumulative.linkValues.forEach((value: any, key: any) => {
                if (!links.find((d: any) => d.fullName === key)) {
                    let parsed = key.split('|')
                    links.push({"source": parsed[0], "target": parsed[1], "value": value, "names": [parsed[0], parsed[1]], "type": parsed[2], "fullName": key})
                }
            });
        }
        links = links.filter((d: any) => d.type === this.measurementMode)

        let linksOriginate = links.filter((entry: any) => {
            return entry.source.includes(this.name)
        })
        let portsOriginate = [...new Set(linksOriginate.map((entry: any) => entry.source).concat(linksOriginate.map((entry: any) => entry.target)))]
        portsOriginate = portsOriginate.map((entry: any) => {return {"name": entry}}).sort((first: any, second: any) => first.name > second.name ? -1 : 1)

        let linksTerminate = links.filter((entry: any) => {
            return entry.target.includes(this.name)
        })
        // Because of how port data is handled, we need to add originating ports so they are registered on the left side
        let portsTerminate = [...new Set(linksTerminate.map((entry: any) => entry.source).concat(linksTerminate.map((entry: any) => entry.target)).concat(linksOriginate.map((entry: any) => entry.source)))]
        portsTerminate = portsTerminate.map((entry: any) => {return {"name": entry}}).sort((first: any, second: any) => first.name > second.name ? -1 : 1)

        // center rect
        canvas
            .append("rect")
            .attr("x", canvasWidth / 3)
            .attr("y", canvasHeight / 3)
            .attr("width", canvasWidth / 3)
            .attr("height", canvasHeight / 3)
            .attr("fill", "Khaki")
            .attr("stroke", "DarkKhaki")
            .attr("stroke-width", "2")
        
        canvas
            .append("text")
            .attr("x", canvasWidth / 2)
            .attr("y", canvasHeight / 4)
            .attr("text-anchor", "middle")
            .text(this.name)
            .attr("style", "font-size: 1.5em")

        if (portsOriginate.length > 0 && linksOriginate.length > 0) {
            let outputsOriginate = sankOriginates({"nodes": portsOriginate, "links": linksOriginate})

            let originateOwnPorts = outputsOriginate.nodes.filter((entry: any) => entry.name.includes(this.name))

            outputsOriginate.nodes = outputsOriginate.nodes.map((entry: any) => {
                let output = entry
                if (entry.name.includes(this.name)) {
                    let height = (canvasHeight / 3 - 4) / originateOwnPorts.length
                    output.y0 = canvasHeight / 3 + (((canvasHeight / 3) / originateOwnPorts.length) * originateOwnPorts.indexOf(entry))
                    output.y1 = output.y0 + height
                } else {
                    let height = output.y1 - output.y0
                    output.y0 = (canvasHeight / 2) + ((output.y0 + (height / 2) - (canvasHeight / 2)) * 3) - height - 10
                    output.y1 = output.y0 + height
                }
                return output
            })
    
            outputsOriginate.links = outputsOriginate.links.map((entry: any) => {
                let output = entry
                output.y1 = (entry.target.y0 + entry.target.y1) / 2
                return output
            })

            canvas.append("g")
                .selectAll("rect")
                .data(outputsOriginate.nodes)
                .join("rect")
                  .attr("x", (d: any) => d.x0)
                  .attr("y", (d: any) => d.y0)
                  .attr("height", (d: any) => d.y1 > d.y0 ? Math.max(d.y1 - d.y0, 1) : 0)
                  .attr("width", (d: any) => d.x1 - d.x0)
                  .attr('fill', 'black')
                  .on('click', (_: Event, d: any) => {
                    let comp = new ComponentDetailView(d.name.substring(0, d.name.lastIndexOf('.')), this.monitor)
                    comp.populate()
                  })
                .append("title")
                  .text((d: any) => `${d.name}`);

            canvas.append("g")
                .attr("fill", "none")
                .selectAll("g")
                .data(outputsOriginate.links.filter((d: any) => d.width > 0))
                .join("path")
                  .attr("d", (d: any) => {
                    let xMidpoint = (d.source.x1 + d.target.x0) / 2
                    let yMidpointTop = (d.source.y0 + d.target.y0*5) / 6
                    let yMidpointBottom = (d.source.y1*5 + d.target.y1) / 6
                    return `M${d.source.x1},${d.source.y0}
                    C${xMidpoint},${d.source.y0},
                    ${xMidpoint},${yMidpointTop},
                    ${d.target.x0},${d.target.y0}
                    L${d.target.x0},${d.target.y1}
                    C${xMidpoint},${d.target.y1},
                    ${xMidpoint},${yMidpointBottom},
                    ${d.source.x1},${d.source.y1}
                    `
                  })
                  .attr("fill", "lightblue")
                  .style("mix-blend-mode", "multiply")
                .append("title")
                  .text((d: any) => `${d.names.join(" → ")}\n${d.value.toLocaleString()}`);

            canvas.append("g")
                .attr("font-size", "1em")
                .selectAll("text")
                .data(outputsOriginate.nodes.filter((item: any) => item.name.includes(this.name)))
                .join("text")
                  .attr("x", (d: any) => d.x0 - 5)
                  .attr("y", (d: any) => (d.y0 + d.y1) / 2 + 4)
                  .attr("text-anchor", "end")
                  .text((d: any) => d.name.slice(d.name.lastIndexOf(".") + 1))
            
            canvas.append("g")
                .attr("font-size", ".66em")
                .attr("font-weight", "bold")
                .selectAll("text")
                .data(outputsOriginate.nodes.filter((item: any) => item.name.includes(this.name)))
                .join("text")
                  .attr("x", (d: any) => d.x1 + 5)
                  .attr("y", (d: any) => (d.y0 + d.y1) / 2 + 4)
                  .attr("text-anchor", "start")
                  .text((d: any) => {
                    let output = d.value
                    if (this.accumulationMode === "Cumulative") {
                        output /= this.sankeyCumulative.epochs.size
                    }
                    if (this.measurementMode === "Msg") {
                        return `${Math.round(output)} Msg/μs`
                    } else {
                        return `${(output * 1e5 / (1 << 30)).toFixed(2)} GB/s`
                    }
                  })

            canvas.append("g")
                .attr("font-size", ".66em")
                .attr("font-weight", "bold")
                .selectAll("text")
                .data(outputsOriginate.nodes.filter((item: any) => !item.name.includes(this.name) && item.y1 > item.y0))
                .join("text")
                  .attr("x", (d: any) => d.x0 - 5)
                  .attr("y", (d: any) => (d.y0 + d.y1) / 2 + 4)
                  .attr("text-anchor", "end")
                  .text((d: any) => d.name)
        }

        if (portsTerminate.length > 0 && linksTerminate.length > 0) {
            let outputsTerminate = sankTermiates({"nodes": portsTerminate, "links": linksTerminate})

            let terminateOwnPorts = outputsTerminate.nodes.filter((entry: any) => entry.name.includes(this.name))

            outputsTerminate.nodes = outputsTerminate.nodes.map((entry: any) => {
                let output = entry
                if (entry.name.includes(this.name)) {
                    let height = (canvasHeight / 3 - 4) / terminateOwnPorts.length
                    output.y0 = canvasHeight / 3 + (((canvasHeight / 3) / terminateOwnPorts.length) * terminateOwnPorts.indexOf(entry))
                    output.y1 = output.y0 + height
                } else {
                    let height = output.y1 - output.y0
                    output.y0 = (canvasHeight / 2) + ((output.y0 + (height / 2) - (canvasHeight / 2)) * 3) - height - 10
                    output.y1 = output.y0 + height
                }
                return output
            })

            outputsTerminate.links = outputsTerminate.links.map((entry: any) => {
                let output = entry
                output.y0 = (entry.source.y0 + entry.source.y1) / 2
                return output
            })

            canvas.append("g")
                .selectAll("rect")
                .data(outputsTerminate.nodes)
                .join("rect")
                  .attr("x", (d: any) => d.x0)
                  .attr("y", (d: any) => d.y0)
                  .attr("height", (d: any) => d.y1 > d.y0 ? Math.max(d.y1 - d.y0, 1) : 0)
                  .attr("width", (d: any) => d.x1 - d.x0)
                  .attr('fill', 'black')
                  .on('click', (_: Event, d: any) => {
                    let comp = new ComponentDetailView(d.name.substring(0, d.name.lastIndexOf('.')), this.monitor)
                    comp.populate()
                  })
                .append("title")
                  .text((d: any) => `${d.name}`);
                  
            canvas.append("g")
                .attr("fill", "none")
                .selectAll("g")
                .data(outputsTerminate.links.filter((d: any) => d.width > 0))
                .join("path")
                  .attr("d", (d: any) => {
                    let xMidpoint = (d.source.x1 + d.target.x0) / 2
                    let yMidpointTop = (d.source.y0 + d.target.y0*5) / 6
                    let yMidpointBottom = (d.source.y1*5 + d.target.y1) / 6
                    return `M${d.source.x1},${d.source.y0}
                    C${xMidpoint},${d.source.y0},
                    ${xMidpoint},${yMidpointTop},
                    ${d.target.x0},${d.target.y0}
                    L${d.target.x0},${d.target.y1}
                    C${xMidpoint},${d.target.y1},
                    ${xMidpoint},${yMidpointBottom},
                    ${d.source.x1},${d.source.y1}
                    `
                  })
                  .attr("fill", "lightgreen")
                  .style("mix-blend-mode", "multiply")
                .append("title")
                  .text((d: any) => `${d.names.join(" → ")}\n${d.value.toLocaleString()}`);
            
            canvas.append("g")
                .attr("font-size", "1em")
                .selectAll("text")
                .data(outputsTerminate.nodes.filter((item: any) => item.name.includes(this.name)))
                .join("text")
                  .attr("x", (d: any) => d.x1 + 5)
                  .attr("y", (d: any) => (d.y0 + d.y1) / 2 + 4)
                  .attr("text-anchor", "start")
                  .text((d: any) => d.name.slice(d.name.lastIndexOf(".") + 1))
            
            canvas.append("g")
                .attr("font-size", ".66em")
                .attr("font-weight", "bold")
                .selectAll("text")
                .data(outputsTerminate.nodes.filter((item: any) => item.name.includes(this.name)))
                .join("text")
                  .attr("x", (d: any) => d.x0 - 5)
                  .attr("y", (d: any) => (d.y0 + d.y1) / 2 + 4)
                  .attr("text-anchor", "end")
                  .text((d: any) => {
                    let output = d.value
                    if (this.accumulationMode === "Cumulative") {
                        output /= this.sankeyCumulative.epochs.size
                    }
                    if (this.measurementMode === "Msg") {
                        return `${Math.round(output)} Msg/μs`
                    } else {
                        return `${(output * 1e5 / (1 << 30)).toFixed(2)} GB/s`
                    }
                  })

            canvas.append("g")
                .attr("font-size", "0.66em")
                .attr("font-weight", "bold")
                .selectAll("text")
                .data(outputsTerminate.nodes.filter((item: any) => !item.name.includes(this.name) && item.y1 > item.y0))
                .join("text")
                  .attr("x", (d: any) => d.x1 + 5)
                  .attr("y", (d: any) => (d.y0 + d.y1) / 2 + 4)
                  .attr("text-anchor", "start")
                  .text((d: any) => d.name)
        }
    }

    groupByLinks(array: any) {
        let outputMap = new Map<string, number>()
        for (const entry of array) {
            let keyString = `${entry.source}|${entry.target}|${entry.type}`
            let thisValue = outputMap.get(keyString)
            if (thisValue === undefined) {
                outputMap.set(keyString, Number.parseInt(entry.value))
            } else {
                outputMap.set(keyString, thisValue + Number.parseInt(entry.value))
            }
        }
        return outputMap
    }
}