import { ComponentDetailView } from "./component"

export function listComponents() {
    fetch("/api/list_components")
        .then(res => res.json())
        .then((res: Array<string>) => {
            res.forEach((compName: string) => {
                console.log(compName)
                let nameTree = compName.split('.')
                let btn = document.createElement("div")
                //initial string
                btn.innerHTML = nameTree[0]
                btn.style.cursor = 'pointer'
                btn.style.userSelect = 'none'
                //additional
                for(let i = 1;i<nameTree.length;i++){ 
                    let newElement = document.createElement('div')
                    newElement.innerHTML = nameTree[i]
                    let indentSize = 25*i
                    let n = indentSize.toString()
                    newElement.style.textIndent = n + 'px'
                    newElement.style.display = 'none'
                    btn.appendChild(newElement)
                }

                function collapse(event:any){
                    const detailView = new ComponentDetailView(compName)
                    detailView.populate()
                    for(let j=0;j<event.target.children.length;j++){
                        if(event.target.children[j].style.display=='block'){
                            event.target.children[j].style.display='none'  
                        }else{
                            event.target.children[j].style.display='block' 
                        }
                    }
                }
                btn.addEventListener("click", collapse)

                document.getElementById('tree-container')
                    .appendChild(btn)
            })
        })
}
