import { ComponentDetailView } from "./component"
import { Monitor } from "./monitor"

const layerIndentation = 25

class Node {
    name: string
    parent: Node | null
    children: Map<string, Node>

    constructor(name: string, parent: Node | null) {
        this.name = name
        this.parent = parent
        this.children = new Map<string, Node>()
    }

    isRoot(): boolean {
        return this.parent === null
    }

    layer(): number {
        let l = 0

        let curr = this.parent
        while (curr != null) {
            l++
            curr = curr.parent
        }

        return l
    }

    fullName(): string {
        let name = this.name

        let curr = this.parent
        while (curr != null) {
            if (curr.name != "") {
                name = curr.name + "." + name
            }
            curr = curr.parent
        }

        return name
    }
}

function addNodeToTree(tree: Node, compName: string) {
    const tokens = compName.split('.')

    let node = tree
    for (let t of tokens) {
        if (!node.children.has(t)) {
            node.children.set(t, new Node(t, node))
        }

        node = node.children.get(t)!
    }
}

function createTree(
    componentNames: Array<string>,
): Node {
    const tree = new Node("", null)

    for (let name of componentNames) {
        addNodeToTree(tree, name)
    }

    return tree
}

function displayDomain(domain: Node, container: HTMLElement, monitor: Monitor) {
    const layer = domain.layer()

    let btn = document.createElement("div")
    btn.innerHTML = `
        <span class="field-title-chevron-right">
            <i class="fa-solid fa-chevron-right fa-xs"></i>
        </span>
        <span class="field-title-chevron-down hidden">
            <i class="fa-solid fa-chevron-down fa-xs" > </i>
        </span>
        <span>${domain.name}</span>`
    btn.style.cursor = 'pointer'
    btn.style.textIndent = ((layer - 1) * layerIndentation) + 'px'
    btn.style.userSelect = 'none'
    container.appendChild(btn)

    const subContainer = document.createElement("div")
    if (domain.isRoot()) {
        subContainer.style.display = 'block'
    } else {
        subContainer.style.display = 'none'
    }
    btn.appendChild(subContainer)

    btn.addEventListener("click", (event: Event) => {
        event.stopImmediatePropagation()
        event.stopPropagation()
        event.preventDefault()

        for (let child of domain.children.values()) {
            display(child, subContainer, monitor)
        }

        if (subContainer.style.display == 'block') {
            subContainer.style.display = 'none'
            btn.querySelector('.field-title-chevron-down')!
                .classList.add('hidden')
            btn.querySelector('.field-title-chevron-right')!
                .classList.remove('hidden')
        } else {
            subContainer.style.display = 'block'
            btn.querySelector('.field-title-chevron-down')!
                .classList.remove('hidden')
            btn.querySelector('.field-title-chevron-right')!
                .classList.add('hidden')
        }
    })
}

function displayComponent(
    component: Node,
    container: HTMLElement,
    monitor: Monitor,
) {
    const layer = component.layer()

    let btn = document.createElement("div")
    btn.innerHTML = `
        <span class="field-title">
            - ${component.name}
        </span>
    `
    btn.style.textIndent = ((layer - 1) * layerIndentation) + 'px'
    btn.style.cursor = 'pointer'

    container.appendChild(btn)

    btn.addEventListener("click", (event: Event) => {
        event.stopImmediatePropagation()
        event.stopPropagation()
        event.preventDefault()

        const detailView = new ComponentDetailView(
            component.fullName(), monitor)
        detailView.populate()
    })
}

function display(
    tree: Node,
    container: HTMLElement,
    monitor: Monitor,
) {
    if (tree.children.size > 0) {
        displayDomain(tree, container, monitor)
    } else {
        displayComponent(tree, container, monitor)
    }
}

export function listComponents(monitor: Monitor) {
    fetch("/api/list_components")
        .then(res => res.json())
        .then((res: Array<string>) => {
            let tree = createTree(res);
            for (let child of tree.children.values()) {
                display(child, document.getElementById("left-pane")!, monitor)
            }
        })
}
