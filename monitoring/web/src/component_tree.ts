import { ComponentDetailView } from "./component"

const layerIndentation = 25

class Node {
    name: string
    parent: Node
    children: Map<string, Node>

    constructor(name: string, parent: Node) {
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

        node = node.children.get(t)
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

function displayDomain(domain: Node, container: HTMLElement) {
    const layer = domain.layer()

    let btn = document.createElement("div")
    btn.innerHTML = domain.name
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

    for (let child of domain.children.values()) {
        display(child, subContainer)
    }

    btn.addEventListener("click", (event: Event) => {
        if (subContainer.style.display == 'block') {
            subContainer.style.display = 'none'
        } else {
            subContainer.style.display = 'block'
        }

        event.stopImmediatePropagation()
        event.stopPropagation()
        event.preventDefault()
    })
}

function displayComponent(
    component: Node,
    container: HTMLElement,
) {
    const layer = component.layer()

    let btn = document.createElement("div")
    btn.innerHTML = component.name
    btn.style.textIndent = ((layer - 1) * layerIndentation) + 'px'
    btn.style.cursor = 'pointer'

    container.appendChild(btn)

    btn.addEventListener("click", (event: Event) => {
        const detailView = new ComponentDetailView(component.fullName())
        detailView.populate()

        event.stopImmediatePropagation()
        event.stopPropagation()
        event.preventDefault()
    })
}

function display(tree: Node, container: HTMLElement) {
    if (tree.children.size > 0) {
        displayDomain(tree, container)
    } else {
        displayComponent(tree, container)
    }
}

export function listComponents() {
    fetch("/api/list_components")
        .then(res => res.json())
        .then((res: Array<string>) => {
            let tree = createTree(res);
            display(tree, document.getElementById('left-pane'))
        })
}
