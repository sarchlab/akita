import { ComponentDetailView } from "./component"

export function listComponents() {
    fetch("/api/list_components")
        .then(res => res.json())
        .then((res: Array<string>) => {
            res.forEach((compName: string) => {
                console.log(compName)

                let btn = document.createElement("div")
                btn.innerHTML = compName
                btn.addEventListener("click",
                    () => {
                        const detailView = new ComponentDetailView(compName)
                        detailView.populate()
                    })

                document.getElementById('tree-container')
                    .appendChild(btn)
            })
        })
}

