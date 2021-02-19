import { UIManager } from "ui_manager"

export class ProgressBarManager {
    uiManager: UIManager
    container: HTMLElement
    pBarDoms: Map<string, ProgressBarDom>

    constructor(uiManager: UIManager) {
        this.pBarDoms = new Map()
        this.uiManager = uiManager
    }

    binDom() {
        this.container = document.getElementById('progress-bar-group')

        window.setInterval(() => this.refreshProgressBars(), 1000)
    }

    refreshProgressBars() {
        fetch('/api/progress').
            then(res => res.json()).
            then((res: Array<ProgressBar>) => {
                if (res == null) {
                    return
                }

                res.forEach((pBar) => {
                    this.showOrUpdateProgressBar(pBar)
                })

                this.removeCompletedProgressBar(res)
            })
    }

    showOrUpdateProgressBar(pBar: ProgressBar) {
        let dom = this.pBarDoms.get(pBar.id)

        if (dom == null) {
            dom = this.createProgressBarDom(pBar)
        }

        dom.set(pBar)
    }

    createProgressBarDom(pBar: ProgressBar): ProgressBarDom {
        const dom = new ProgressBarDom(pBar)

        this.pBarDoms.set(pBar.id, dom)

        dom.show(this.container, this.uiManager)

        return dom
    }

    removeCompletedProgressBar(pBars: Array<ProgressBar>) {
        this.pBarDoms.forEach((dom, key) => {
            let found = false
            for (let i = 0; i < pBars.length; i++) {
                if (pBars[i].id == key) {
                    found = true
                }
            }

            if (!found) {
                this.pBarDoms.delete(key)
                dom.remove().then(() => {
                    this.uiManager.resize()
                })
            }
        })
    }
}

class ProgressBarDom {
    dom: HTMLElement
    label: HTMLElement
    progressBar: HTMLElement
    finishedDom: HTMLElement
    inProgressDom: HTMLElement
    unfinishedDom: HTMLElement

    constructor(pBar: ProgressBar) {
        this.dom = document.createElement('div')
        this.dom.classList.add('progress-complex')

        this.label = document.createElement('label')
        this.label.innerHTML = pBar.name
        this.dom.appendChild(this.label)

        this.progressBar = document.createElement('div')
        this.progressBar.classList.add('progress', 'ml-3')

        this.finishedDom = document.createElement('div')
        this.finishedDom.classList.add('progress-bar', 'bg-success')

        this.inProgressDom = document.createElement('div')
        this.inProgressDom.classList.add(
            'progress-bar', 'progress-bar-striped')

        this.unfinishedDom = document.createElement('div')
        this.unfinishedDom.classList.add(
            'progress-bar', 'bg-secondary')

        this.progressBar.appendChild(this.finishedDom)
        this.progressBar.appendChild(this.inProgressDom)
        this.progressBar.appendChild(this.unfinishedDom)

        this.dom.appendChild(this.progressBar)
    }

    async show(container: HTMLElement, uiManager: UIManager) {
        container.appendChild(this.dom)
        uiManager.resize()
        this.resize()

        const distance = window.innerHeight - this.dom.offsetTop

        this.dom.animate(
            { transform: `translate(0, ${distance}px)` },
            { duration: 200, easing: "ease-in-out", direction: "reverse" }
        )
    }

    resize() {
        this.progressBar.style.width =
            `${this.dom.offsetWidth - this.label.offsetWidth - 20}px`
    }

    set(pBar: ProgressBar) {
        if (!pBar.finished) {
            pBar.finished = 0
        }
        this.finishedDom.style.width =
            `${pBar.finished / pBar.total * 100}%`
        this.finishedDom.innerHTML = `${pBar.finished}`

        this.inProgressDom.style.width =
            `${pBar.in_progress / pBar.total * 100}%`
        this.inProgressDom.innerHTML = `${pBar.in_progress}`

        const unfinished = pBar.total - pBar.in_progress - pBar.finished
        this.unfinishedDom.style.width =
            `${unfinished / pBar.total * 100}%`
        this.unfinishedDom.innerHTML = `${unfinished}`
    }

    async remove(): Promise<void> {
        const distance = window.innerHeight - this.dom.offsetTop;

        return this.dom.
            animate(
                { transform: `translate(0, ${distance}px)` },
                { duration: 200, easing: "ease-in-out" }
            ).finished.
            then(
                () => {
                    this.dom.remove()
                }
            )
    }
}

interface ProgressBar {
    id: string
    name: string
    start_time: string
    total: number
    in_progress: number
    finished: number
}
