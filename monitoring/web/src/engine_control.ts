export class EngineController {
    continueButton: HTMLElement | null = null
    pauseButton: HTMLElement | null = null
    runButton: HTMLElement | null = null
    nowLabel: HTMLElement | null = null

    bindDom() {
        this.runButton = document.getElementById('run-button')
        this.continueButton = document.getElementById('continue-button')
        this.pauseButton = document.getElementById('pause-button')
        this.nowLabel = document.getElementById('now-label')

        window.setInterval(() => { this.updateTime() }, 1000)

        this.runButton!.addEventListener('click', (e: Event) => {
            this.run(e)
        })
        this.continueButton!.addEventListener('click', (e: Event) => {
            this.continue(e)
        })
        this.pauseButton!.addEventListener('click', (e: Event) => {
            this.pause(e)
        })
    }

    updateTime() {
        fetch('/api/now')
            .then(res => res.json())
            .then((res: any) => {
                this.nowLabel!.innerHTML = res.now
            })
    }

    run(e: Event) {
        fetch('/api/run')

        e.stopImmediatePropagation()
        e.stopPropagation()
        e.preventDefault()
    }

    pause(e: Event) {
        fetch('/api/pause')

        e.stopImmediatePropagation()
        e.stopPropagation()
        e.preventDefault()
    }

    continue(e: Event) {
        fetch('/api/continue')

        e.stopImmediatePropagation()
        e.stopPropagation()
        e.preventDefault()
    }

}