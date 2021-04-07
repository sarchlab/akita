import { ComponentDetailView } from "./component"

export function listComponents() {
    fetch("/api/list_components")
        .then(res => res.json())
        .then((res: Array<string>) => {
            //Use recursion to develop tree like structure with maps, every 'tree' is a map, leafs are represented as part of an array
            function createTree(arr:any){
                let containsPeriod = false
                for(let i of arr){
                    if(i.includes('.')){
                        containsPeriod = true
                        break
                    }
                }
                if(containsPeriod){
                    let curr = arr
                    arr = new Map()
                    for(let c of curr){
                        let split: any
                        if(c.includes('.')){
                            split = [c.substring(0, c.indexOf('.')), c.substring(c.indexOf('.')+1)]
                        }else{
                            split = [c]
                        }
                        if(arr.has(split[0])){
                            let p = arr.get(split[0]).push(split[1])
                        } else{
                            if(split.length>1){
                                arr.set(split[0], [split[1]])
                            } else{
                                arr.set(split[0], [split[0]])
                            }
                        }
                    }
                    //if there is an array, recursively call the function to convert it to a map
                    for(let d of arr.keys()){
                        if(Array.isArray(arr.get(d))){
                            arr.set(d, createTree(arr.get(d)))
                        }
                    }
                    return arr
                }else{
                    return arr
                }
            }
            let newStruct = createTree(res);
            console.log(newStruct)
            function display(struct:any, indent:any, graphic:any, strng:any){
                for(let name of struct.keys()){
                    //If the next 'node' is a map
                    if(struct.get(name) instanceof Map){
                        let btn = document.createElement("div")
                        //initial string
                        btn.innerHTML = name
                        btn.style.cursor = 'pointer'
                        btn.style.textIndent = indent + 'px'
                        if(indent==0){
                            btn.style.display = 'block'
                        }else{
                            btn.style.display = 'none'
                        }
                        btn.style.userSelect = 'none'
                        function collapse(event:any){
                            for(let j=0;j<event.target.children.length;j++){
                                if(event.target.children[j].style.display=='block'){
                                    event.target.children[j].style.display='none'  
                                }else{
                                    event.target.children[j].style.display='block' 
                                }
                            }
                        }
                        btn.addEventListener("click", collapse)
                        graphic.appendChild(btn)
                        if(strng!=''){
                            display(struct.get(name), indent+25, btn, strng+'.'+name)
                        }else{
                            display(struct.get(name), indent+25, btn, name)
                        }
                    //If the next 'node' is an array
                    }else{
                        let btn = document.createElement("div")
                        //initial string
                        btn.innerHTML = name
                        btn.style.textIndent = indent + 'px'
                        btn.style.cursor = 'pointer'
                        if(indent==0){
                            btn.style.display = 'block'
                        }else{
                            btn.style.display = 'none'
                        }
                        function collapseInner(event:any){
                            //This is things like driver and MMU
                            if(indent==0){
                                const detailView = new ComponentDetailView(name)
                                detailView.populate()
                            //If the indent is greater than 0, check whether the array length is < 2
                            }else if(struct.get(name).length<2){
                                const detailView = new ComponentDetailView(strng+'.'+name)
                                detailView.populate()
                            }
                            for(let j=0;j<event.target.children.length;j++){
                                if(event.target.children[j].style.display=='block'){
                                    event.target.children[j].style.display='none !important'    
                                }else{
                                    event.target.children[j].style.display='block !important' 
                                }
                            }
                        }
                        btn.addEventListener("click", collapseInner)
                        graphic.appendChild(btn)
                        //Create the last set of leafs, if the array is > 1
                        if(struct.get(name).length>=2 && struct.get(name)[0] != name){
                            for(let com of struct.get(name)){
                                let btnInner = document.createElement("div")
                                //initial string
                                btnInner.innerHTML = com
                                btnInner.style.display = 'none'
                                btnInner.style.cursor = 'pointer'
                                btnInner.style.textIndent = (indent+25) + 'px'
                                let compName = strng+'.'+name+'.'+com
                                function displayComp(){
                                    console.log(compName)
                                    const detailView = new ComponentDetailView(compName)
                                    detailView.populate()
                                }
                                btnInner.addEventListener("click", displayComp)
                                btn.appendChild(btnInner)
                            }
                        }   
                    }
                }
            }
            display(newStruct, 0, document.getElementById('tree-container'), '') 
        })
}
