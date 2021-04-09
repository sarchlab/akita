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


    // //If the next 'node' is a map
    // if (tree.get(name) instanceof Map) {

    //     //If the next 'node' is an array
    // } else {
    //     let btn = document.createElement("div")
    //     //initial string
    //     btn.innerHTML = name
    //     btn.style.textIndent = indent + 'px'
    //     btn.style.cursor = 'pointer'
    //     if (indent == 0) {
    //         btn.style.display = 'block'
    //     } else {
    //         btn.style.display = 'none'
    //     }
    //     function collapseInner(event: any) {
    //         //This is things like driver and MMU
    //         if (indent == 0) {
    //             const detailView = new ComponentDetailView(name)
    //             detailView.populate()
    //             //If the indent is greater than 0, check whether the array length is < 2
    //         } else if (tree.get(name).length < 2) {
    //             const detailView = new ComponentDetailView(strng + '.' + name)
    //             detailView.populate()
    //         }
    //         for (let j = 0; j < event.target.children.length; j++) {
    //             if (event.target.children[j].style.display == 'block') {
    //                 event.target.children[j].style.display = 'none !important'
    //             } else {
    //                 event.target.children[j].style.display = 'block !important'
    //             }
    //         }
    //     }
    //     btn.addEventListener("click", collapseInner)
    //     container.appendChild(btn)
    //     //Create the last set of leafs, if the array is > 1
    //     if (tree.get(name).length >= 2 && tree.get(name)[0] != name) {
    //         for (let com of tree.get(name)) {
    //             let btnInner = document.createElement("div")
    //             //initial string
    //             btnInner.innerHTML = com
    //             btnInner.style.display = 'none'
    //             btnInner.style.cursor = 'pointer'
    //             btnInner.style.textIndent = (indent + 25) + 'px'
    //             let compName = strng + '.' + name + '.' + com
    //             function displayComp() {
    //                 console.log(compName)
    //                 const detailView = new ComponentDetailView(compName)
    //                 detailView.populate()
    //             }
    //             btnInner.addEventListener("click", displayComp)
    //             btn.appendChild(btnInner)
    //         }
    //     }
    // }
}

export function listComponents() {
    fetch("/api/list_components")
        .then(res => res.json())
        .then((res: Array<string>) => {
            let tree = createTree(res);
            display(tree, document.getElementById('tree-container'))
        })
}
