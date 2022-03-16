export class ComponentDetailView {
    name: string

    constructor(name: string) {
        this.name = name
    }

    populate() {
        fetch(`/api/component/${this.name}`)
            .then(res => res.json())
            .then((res: any) => {
                const container = document.getElementById('central-pane')
                const object = res["dict"][res["root"]]

                this.showComponent(object, container)
            })
    }

    showComponent(comp: any, container: HTMLElement) {
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

        this.showContent(comp, componentDetailContainer, "")
    }

    showContent(s: any, container: HTMLElement, fieldPrefix: string) {
        container.innerHTML = ""

        if (Array.isArray(s)) {
            this.showSlice(s, container, fieldPrefix)
            return
        }

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
                this.showValue(s['v'], container)
                break;
            case 25:
                this.showStruct(s['v'], container, fieldPrefix)
                break;
            default:
                console.error(`value kind ${s['k']} is not supported`)
        }
    }

    showValue(s: any, container: HTMLElement) {
        container.innerHTML = s
    }

    showSlice(s: Array<any>, container: HTMLElement, fieldPrefix: string) {
        const table = document.createElement('table');
        table.classList.add('detail-table')
        container.appendChild(table)

        for (let i = 0; i < s.length; i++) {
            const item = s[i]

            const row = document.createElement('tr')
            const cell = document.createElement('td')
            cell.innerHTML = `
                <div class="field_name">${i}</div>
            `;

            const valueContainer = document.createElement('div')
            valueContainer.classList.add("field-value")
            cell.appendChild(valueContainer)


            cell.addEventListener('click', (e: Event) => {
                e.stopPropagation()

                if (valueContainer.innerHTML != "") {
                    valueContainer.innerHTML = ""
                    return
                }

                this.showField(`${fieldPrefix}${i}`, valueContainer)
            })

            row.appendChild(cell)
            table.appendChild(row)
        }
    }

    showStruct(s: any, container: HTMLElement, fieldPrefix: string) {
        const table = document.createElement('table');
        table.classList.add('detail-table')
        container.appendChild(table)

        let fields = Object.keys(s);
        fields = fields.sort()

        fields.forEach(f => {
            const row = document.createElement('tr')
            const cell = document.createElement('td')
            cell.innerHTML = `
                <div class="field_name">${f}</div>
            `;

            const valueContainer = document.createElement('div')
            valueContainer.classList.add("field-value")
            cell.appendChild(valueContainer)


            cell.addEventListener('click', (e: Event) => {
                e.stopPropagation()

                if (valueContainer.innerHTML != "") {
                    valueContainer.innerHTML = ""
                    return
                }

                this.showField(`${fieldPrefix}${f}`, valueContainer)
            })

            row.appendChild(cell)
            table.appendChild(row)
        });
    }

    showField(field: string, container: HTMLElement) {
        const req = {
            comp_name: this.name,
            field_name: field,
        }
        fetch(`/api/field/${JSON.stringify(req)}`)
            .then(res => res.json())
            .then(res => {
                this.showContent(res["dict"][res["root"]], container,
                    `${field}.`)
            })
    }
}