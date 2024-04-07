import { VarKind, isContainerKind, isDirectKind } from "./gotypes"
import { Monitor } from "./monitor"
import * as d3 from "d3"
import * as sankey from "d3-sankey"

export class ComponentDetailView {
    name: string
    monitor: Monitor

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
            })

            fetch(`/api/traffic/${this.name}`)
            .then(res => res.json())
            .then((res: any) => {
                const container = document.getElementById('right-pane')

                this.showSankey(res, container!)
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

    showSankey(dict: any, container: HTMLElement) {
        let hangAnalyzerBtn = document.querySelector('.auto-refresh-btn')
        if (hangAnalyzerBtn !== null && hangAnalyzerBtn.classList.contains('btn-primary')) {
            hangAnalyzerBtn.click()
        }

        container.innerHTML = ""

        const sankeyContainer = document.createElement("div");
        sankeyContainer.classList.add("sankey-container")
        container.appendChild(sankeyContainer)

        const sankeyHeader = document.createElement("div")
        sankeyHeader.classList.add("sankey-header")
        sankeyHeader.innerHTML = `
            <div class="sankey-name">${this.name} Data Flows</div>
        `

        sankeyContainer.appendChild(sankeyHeader)

        if (dict.length === 0) {
            sankeyHeader.innerHTML = `
            <div class="sankey-name">No Data for ${this.name}</div>`
            return
        }

        const canvas = document.createElement('svg')
        canvas.classList.add("sankey-diagram")
        sankeyContainer.appendChild(canvas)

        canvas.setAttribute('width', '680')
        canvas.setAttribute('height', '370')

        this.drawSankey(dict, canvas)
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

    drawSankey(data: any, target: HTMLElement) {
        const canvas = d3.select<HTMLElement, unknown>(target)

        const canvasDims = target.getBoundingClientRect()
        const canvasHeight = canvasDims.height
        const canvasWidth = canvasDims.width

        const sankGen = sankey.sankey()
        .nodeWidth(4)
        .nodePadding(20)
        .extent([[0, 0], [canvasWidth, canvasHeight]])
        .nodeId((d: any) => d.name)

        let dataSpecified = data.map((entry: any) => {
            let output = entry
            output.localPort = entry.localPort + " (Source)"
            output.remotePort = entry.remotePort + " (Destination)"
            return output
        })

        // Grab every unique port
        let ports = [...new Set(dataSpecified.map((entry: any) => entry.localPort).concat(dataSpecified.map((entry: any) => entry.remotePort)))]
        ports = ports.map((entry: any) => {return {"name": entry}})

        let linksRaw = dataSpecified.map((entry: any) => {return {"source": entry.localPort, "target": entry.remotePort, "value": entry.value}}).filter((entry: any) => entry.value > 0)
        // There's a Map.groupBy function, but it doesn't work in safari
        let linksMap = this.groupByLinks(linksRaw)
        let links = []
        for (let entry of linksMap.entries()) {
            let parsed = entry[0].split('|')
            let value = entry[1]
            links.push({"source": parsed[0], "target": parsed[1], "value": value, "names": parsed})
        }

        let outputs = sankGen({"nodes": ports, "links": links})

        const color = d3.scaleOrdinal(["Perished"], ["#da4f81"]).unknown("#ccc");

        canvas.append("g")
            .selectAll("rect")
            .data(outputs.nodes)
            .join("rect")
              .attr("x", (d: any) => d.x0)
              .attr("y", (d: any) => d.y0)
              .attr("height", (d: any) => d.y1 - d.y0)
              .attr("width", (d: any) => d.x1 - d.x0)
              .attr("stroke", 'black')
              .attr("stroke-width", "2")
              .attr('fill', 'black')
            .append("title")
              .text((d: any) => `${d.name}`);

        canvas.append("g")
            .attr("fill", "none")
            .selectAll("g")
            .data(outputs.links)
            .join("path")
              .attr("d", sankey.sankeyLinkHorizontal())
              .attr("stroke", (d: any) => color(d.names[0]))
              .attr("stroke-width", (d: any) => d.width)
              .style("mix-blend-mode", "multiply")
            .append("title")
              .text((d: any) => `${d.names.join(" → ")}\n${d.value.toLocaleString()}`);

        canvas.append("g")
            .style("font", "10px sans-serif")
            .selectAll("text")
            .data(outputs.nodes)
            .join("text")
                .attr("x", (d: any) => d.x0 < canvasWidth / 2 ? d.x1 + 6 : d.x0 - 6)
                .attr("y", (d: any) => (d.y1 + d.y0) / 2)
                .attr("dy", "0.35em")
                .attr("text-anchor", (d: any) => d.x0 < canvasWidth / 2 ? "start" : "end")
                .text((d: any) => d.name)
            .append("tspan")
                .attr("fill-opacity", 0.7)
                .text((d: any) => ` ${d.value.toLocaleString()}`);
    }

    groupByLinks(array: any) {
        let outputMap = new Map<string, number>()
        for (const entry of array) {
            let keyString = `${entry.source}|${entry.target}`
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