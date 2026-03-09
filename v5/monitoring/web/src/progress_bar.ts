import { UIManager } from "./ui_manager"

export class ProgressBarManager {
    uiManager: UIManager
    container: HTMLElement | null = null
    progressDoms: Map<string, ProgressDom>
    mode = 'bar'
    maxProgressBars = 2

    constructor(uiManager: UIManager) {
        this.progressDoms = new Map()
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

                this.renderProgress(res)
            })
    }

    renderProgress(pBars: Array<ProgressBar>) {
        if (this.mode == 'bar' && pBars.length > this.maxProgressBars) {
            this.mode = 'pie'
            this.removeAllProgressDoms()
        }

        if (this.mode == 'pie' && pBars.length <= this.maxProgressBars) {
            this.mode = 'bar'
            this.removeAllProgressDoms()
        }



        if (this.mode == 'bar') {
            this.renderProgressAsBars(pBars)
        }

        if (this.mode == 'pie') {
            this.renderProgressAsPies(pBars)
        }
    }

    renderProgressAsBars(pBars: Array<ProgressBar>) {
        pBars.forEach((pBar) => {
            this.showOrUpdateProgressBarAsBar(pBar)
        })

        this.removeCompletedProgressDoms(pBars)
    }

    renderProgressAsPies(pBars: Array<ProgressBar>) {
        pBars.forEach((pBar) => {
            this.showOrUpdateProgressBarAsPie(pBar)
        })

        this.removeCompletedProgressDoms(pBars)
    }

    showOrUpdateProgressBarAsBar(pBar: ProgressBar) {
        let dom = this.progressDoms.get(pBar.id)

        if (dom == null) {
            dom = this.createProgressBarDom(pBar)
            this.uiManager.adjustProgressBarGroupHeight()
        }

        dom.set(pBar)
    }

    showOrUpdateProgressBarAsPie(pBar: ProgressBar) {
        let dom = this.progressDoms.get(pBar.id)

        if (dom == null) {
            dom = this.createProgressPieDom(pBar)
            this.uiManager.adjustProgressBarGroupHeight()
        }

        dom.set(pBar)
    }


    createProgressBarDom(pBar: ProgressBar): ProgressBarDom {
        const dom = new ProgressBarDom(pBar)

        this.progressDoms.set(pBar.id, dom)

        dom.show(this.container!, this.uiManager)

        return dom
    }

    createProgressPieDom(pBar: ProgressBar): ProgressPieDom {
        const dom = new ProgressPieDom(pBar)

        this.progressDoms.set(pBar.id, dom)

        dom.show(this.container!)

        return dom
    }

    removeAllProgressDoms() {
        this.progressDoms.forEach((dom, key) => {
            this.progressDoms.delete(key)
            dom.remove().then(() => {
                this.uiManager.adjustProgressBarGroupHeight()
            })
        })
    }

    removeCompletedProgressDoms(pBars: Array<ProgressBar>) {
        this.progressDoms.forEach((dom, key) => {
            let found = false
            for (let i = 0; i < pBars.length; i++) {
                if (pBars[i].id == key) {
                    found = true
                }
            }

            if (!found) {
                this.progressDoms.delete(key)
                dom.remove().then(() => {
                    this.uiManager.adjustProgressBarGroupHeight()
                })
            }
        })
    }
}

interface ProgressDom {
    set(pBar: ProgressBar): void
    remove(): Promise<void>
    show(container: HTMLElement, uiManager: UIManager): void
}

class ProgressPieDom {
    dom: HTMLElement
    tooltip: HTMLElement | null = null
    progressPie: HTMLElement

    constructor(pBar: ProgressBar) {
        this.dom = document.createElement('div')
        this.dom.classList.add('progress-pie-complex')

        this.progressPie = document.createElement('div')
        this.progressPie.classList.add('progress-pie')
        this.dom.appendChild(this.progressPie)

        this.createTooltip(pBar)
    }

    private createTooltip(pBar: ProgressBar) {
        this.tooltip = document.createElement('div')
        this.tooltip.classList.add('progress-pie-tooltip')
        this.tooltip.innerHTML = `
            <div class="progress-pie-tooltip-title">${pBar.name}</div>
            <div class="progress-pie-tooltip-content">
                Finished: ${pBar.finished},
                In Progress: ${pBar.in_progress},
                Total: ${pBar.total}
            </div>
        `
        this.dom.appendChild(this.tooltip)

        this.dom.addEventListener('mouseover', () => {
            this.tooltip!.style.display = 'block'
        })

        this.dom.addEventListener('mousemove', (e) => {
            this.tooltip!.style.left = `${e.clientX}px`
            this.tooltip!.style.top = `${e.clientY - this.tooltip!.offsetHeight}px`
        })

        this.dom.addEventListener('mouseout', () => {
            this.tooltip!.style.display = 'none'
        })
    }

    async show(container: HTMLElement) {
        container.appendChild(this.dom)

        await this.dom.animate(
            { transform: `scale(0)` },
            { duration: 200, easing: "ease-in-out" }
        ).finished
    }

    resize() {

    }

    async remove() {
        const distance = window.innerHeight - this.dom.offsetTop

        await this.dom.animate(
            { transform: `translate(0, ${distance}px)` },
            { duration: 200, easing: "ease-in-out" }
        ).finished

        this.dom.remove()
    }

    set(pBar: ProgressBar) {
        const finished = pBar.finished
        const inProgress = pBar.in_progress
        const total = pBar.total

        const finishedPercent = finished / total
        const finishedDegree = finishedPercent * 360
        const inProgressPercent = inProgress / total
        const inProgressDegree = inProgressPercent * 360

        this.progressPie.style.backgroundImage = `
            conic-gradient(
                #28a745 ${finishedDegree}deg,
                #007bff ${finishedDegree}deg ${finishedDegree + inProgressDegree}deg,
                #e9ecef ${finishedDegree + inProgressDegree}deg
            )
        `
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
