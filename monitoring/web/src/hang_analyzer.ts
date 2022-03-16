export class HangAnalyzer {
	running: boolean = false
	intervalHandle: number

	bindDom() {
		const btn = document.getElementById("hang-analyzer-btn")
		btn.addEventListener('click', (e: Event) => {
			this.startAnalyzing()

			e.preventDefault()
			e.stopImmediatePropagation()
			e.stopPropagation()
		})
	}

	startAnalyzing() {
		this.showBufferList()

		// if (!this.running) {
		// 	this.intervalHandle = window.setInterval(() => {
		// 		this.showBufferList()
		// 	}, 1000)
		// 	this.running = true
		// }
	}

	stopAnalyzing() {
		if (this.running) {
			window.clearInterval(this.intervalHandle)
			this.running = false
		}
	}

	showBufferList() {
		fetch('/api/hangdetector/buffers')
			.then(res => res.json())
			.then((res: any) => {
				const container = document.getElementById('right-pane')
				container.innerHTML = ""

				const bufferList = document.createElement("table")
				bufferList.classList.add("table", "buffer-table")
				container.appendChild(bufferList)

				const header = document.createElement("tr")
				header.innerHTML = `
					<th>Buffer</th>
					<th class='buffer-value'>Size</th>
					<th class='buffer-value'>Cap</th>
				`
				bufferList.appendChild(header)

				res.forEach((buffer: any) => {
					const bufferItem = document.createElement("tr")
					bufferItem.classList.add("buffer-item")
					bufferItem.innerHTML = `
						<td class="buffer-name">${buffer.buffer}</td>
						<td class="buffer-value">${buffer.level}</td>
						<td class="buffer-value">${buffer.cap}</td>
					`
					bufferList.appendChild(bufferItem)
				})
			});
	}
}