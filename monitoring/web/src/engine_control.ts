export class EngineController {
    continueButton: HTMLElement | null = null
    pauseButton: HTMLElement | null = null
    runButton: HTMLElement | null = null
    nowLabel: HTMLElement | null = null
    stepButton: HTMLElement | null = null
    stepAmountInput: HTMLInputElement | null = null
    stepUnitSelect: HTMLSelectElement | null = null
    stepDropdownButton: HTMLElement | null = null
    stepPanel: HTMLElement | null = null

    bindDom() {
        this.runButton = document.getElementById('run-button')
        this.continueButton = document.getElementById('continue-button')
        this.pauseButton = document.getElementById('pause-button')
        this.nowLabel = document.getElementById('now-label')
        this.stepButton = document.getElementById('step-button')
        this.stepAmountInput = document.getElementById('step-amount') as HTMLInputElement
        this.stepUnitSelect = document.getElementById('step-unit') as HTMLSelectElement
        this.stepDropdownButton = document.getElementById('step-dropdown-button')
        this.stepPanel = document.getElementById('step-panel')

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
        this.stepButton!.addEventListener('click', (e: Event) => {
            this.step(e)
        })
        this.stepDropdownButton!.addEventListener('click', (e: Event) => {
            this.toggleStepPanel(e)
        })

        // Close dropdown when clicking outside
        document.addEventListener('click', (e: Event) => {
            if (!this.stepDropdownButton!.contains(e.target as Node) && 
                !this.stepPanel!.contains(e.target as Node)) {
                this.hideStepPanel()
            }
        })
    }

    updateTime() {
        fetch('/api/now')
            .then(res => res.json())
            .then((res: any) => {
                this.nowLabel!.innerHTML = res.now
            })
    }

    toggleStepPanel(e: Event) {
        const isVisible = this.stepPanel!.style.display !== 'none'
        if (isVisible) {
            this.hideStepPanel()
        } else {
            this.showStepPanel()
        }
        
        e.stopImmediatePropagation()
        e.stopPropagation()
        e.preventDefault()
    }

    showStepPanel() {
        this.stepPanel!.style.display = 'block'
    }

    hideStepPanel() {
        this.stepPanel!.style.display = 'none'
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

    step(e: Event) {
        const amount = parseFloat(this.stepAmountInput!.value)
        const unit = this.stepUnitSelect!.value

        if (isNaN(amount) || amount <= 0) {
            alert('Please enter a valid positive number for step amount')
            return
        }

        const stepData = {
            amount: amount,
            unit: unit
        }

        fetch('/api/step', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(stepData)
        })
        .then(response => {
            if (!response.ok) {
                return response.text().then(text => {
                    throw new Error(`Step failed: ${text}`)
                })
            }
            // Close the panel after successful step
            this.hideStepPanel()
        })
        .catch(error => {
            console.error('Step error:', error)
            alert(`Step failed: ${error.message}`)
        })

        e.stopImmediatePropagation()
        e.stopPropagation()
        e.preventDefault()
    }

}