import { VarKind, isContainerKind, isDirectKind } from "./gotypes"
import { Monitor } from "./monitor"

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

        this.showContent(comp, dict, "", componentDetailContainer, "")
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
}