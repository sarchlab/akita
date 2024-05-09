export class HangAnalyzer {
	running: boolean = false
	sort = "level"
	intervalHandle: number = 0

	bindDom() {
		const btn = document.getElementById("hang-analyzer-btn")!
		btn.addEventListener('click', (e: Event) => {
			this.prepareDom()
			this.startAnalyzing()

			e.preventDefault()
			e.stopImmediatePropagation()
			e.stopPropagation()
		})
	}

	prepareDom() {
		let sankeyAnalyzerBtn = document.querySelector<HTMLElement>('.auto-sankey-refresh-btn')
        if (sankeyAnalyzerBtn !== null && sankeyAnalyzerBtn.classList.contains('btn-primary')) {
            sankeyAnalyzerBtn.click()
        }

		const container = document.getElementById('right-pane')!
		container.innerHTML = ""

		const toolbar = document.createElement("div")
		toolbar.classList.add("btn-toolbar", "mb-3")
		container.appendChild(toolbar)

		const sortOptions = document.createElement("div")
		sortOptions.classList.add("input-group")
		sortOptions.setAttribute("role", "group")
		sortOptions.innerHTML = `<div class="input-group-text">Sort by:</div>`
		toolbar.appendChild(sortOptions)

		const bufferTable = document.createElement("table")
		bufferTable.classList.add("table", "buffer-table")
		bufferTable.setAttribute("id", "buffer-table")
		container.appendChild(bufferTable)

		const sortBySizeBtn = document.createElement("button")
		sortBySizeBtn.classList.add("btn", "btn-primary")
		sortBySizeBtn.innerHTML = "Size"
		sortOptions.appendChild(sortBySizeBtn)

		const sortByPercentBtn = document.createElement("button")
		sortByPercentBtn.classList.add("btn", "btn-outline-primary")
		sortOptions.appendChild(sortByPercentBtn)
		sortByPercentBtn.innerHTML = "Percent"

		sortBySizeBtn.addEventListener("click", (_: Event) => {
			this.sort = "level"

			this.showBufferList()

			sortBySizeBtn.classList.remove('btn-outline-primary')
			sortBySizeBtn.classList.add('btn-primary')
			sortByPercentBtn.classList.remove('btn-primary')
			sortByPercentBtn.classList.add('btn-outline-primary')

		})

		sortByPercentBtn.addEventListener("click", (_: Event) => {
			this.sort = "percent"
			this.showBufferList()

			sortByPercentBtn.classList.remove('btn-outline-primary')
			sortByPercentBtn.classList.add('btn-primary')
			sortBySizeBtn.classList.remove('btn-primary')
			sortBySizeBtn.classList.add('btn-outline-primary')
		})

		const autoRefreshBtn = document.createElement("button")
		autoRefreshBtn.classList.add("btn", "btn-primary", "ms-3", "auto-refresh-btn")
		autoRefreshBtn.innerHTML = "Stop Refresh"
		toolbar.appendChild(autoRefreshBtn)

		autoRefreshBtn.addEventListener("click", (_: Event) => {
			if (this.running) {
				this.stopAnalyzing()
				autoRefreshBtn.classList.remove('btn-primary')
				autoRefreshBtn.classList.add('btn-outline-primary')
				autoRefreshBtn.innerHTML = "Auto Refresh"
			} else {
				this.startAnalyzing()
				autoRefreshBtn.classList.remove('btn-outline-primary')
				autoRefreshBtn.classList.add('btn-primary')
				autoRefreshBtn.innerHTML = "Stop Refresh"
			}
		})
	}

	startAnalyzing() {
		this.showBufferList()

		if (!this.running) {
			this.intervalHandle = window.setInterval(() => {
				this.showBufferList()
			}, 2000)
			this.running = true
		}
	}

	stopAnalyzing() {
		if (this.running) {
			window.clearInterval(this.intervalHandle)
			this.running = false
		}
	}

	showBufferList() {

		fetch(`/api/hangdetector/buffers?sort=${this.sort}&limit=40`)
			.then(res => res.json())
			.then((res: any) => {
				const bufferTable = document.getElementById("buffer-table")!
				bufferTable.innerHTML = ""

				const header = document.createElement("tr")
				header.innerHTML = `
						<th> Buffer </th>
						<th class='buffer-value'> Size </th>
							<th class='buffer-value'> Cap </th>
					`
				bufferTable.appendChild(header)

				res.forEach((buffer: any) => {
					const bufferItem = document.createElement("tr")
					bufferItem.classList.add("buffer-item")
					bufferItem.innerHTML = `
					<td class="buffer-name"> ${buffer.buffer} </td>
						<td class="buffer-value"> ${buffer.level} </td>
							<td class="buffer-value"> ${buffer.cap} </td>
								`
					bufferTable.appendChild(bufferItem)
				})
			});
	}
}