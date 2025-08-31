import './reset.css'
import './styles.scss'
import './style.css'

import '@fortawesome/fontawesome-free/js/fontawesome'
import '@fortawesome/fontawesome-free/js/solid'
import '@fortawesome/fontawesome-free/js/regular'
// import '@fortawesome/fontawesome-free/js/brands'

import './component_tree'
import { UIManager } from './ui_manager'
import { listComponents } from './component_tree'
import { EngineController } from './engine_control'
import { ProgressBarManager } from './progress_bar'
import { HangAnalyzer } from './hang_analyzer'
import { Monitor } from './monitor'
import { ResourceMonitor } from './resource'

class App {
    uiManager: UIManager
    monitor: Monitor
    resourceMonitor: ResourceMonitor
    engineController: EngineController
    progressBarManager: ProgressBarManager
    hangAnalyzer: HangAnalyzer

    constructor() {
        this.uiManager = new UIManager();
        this.resourceMonitor = new ResourceMonitor();
        this.engineController = new EngineController();
        this.progressBarManager = new ProgressBarManager(this.uiManager);
        this.hangAnalyzer = new HangAnalyzer();
        this.monitor = new Monitor(this.uiManager);
    }

    start() {
        this.uiManager.assignElements()
        this.uiManager.resize()

        this.engineController.bindDom()
        this.progressBarManager.binDom()
        this.hangAnalyzer.bindDom()

        window.addEventListener("resize", () => {
            this.uiManager.resize();
        });

        listComponents(this.monitor)

        // --- Trace Toggle Button Logic ---
        const traceBtn = document.getElementById("trace-toggle-btn") as HTMLButtonElement;

        // Fetch the initial tracing status from the backend
        fetch("/api/trace/is_tracing")
            .then((response) => response.json())
            .then((data) => {
                let tracing = data.isTracing; // Initialize tracing state

                function updateTraceBtn(tracing: boolean) {
                    const iconStyle = 'width: 24px; height: 24px; margin-right: 8px; background-color: transparent; border: none; filter: brightness(0) invert(1);';
                    if (tracing) {
                        traceBtn.classList.add("btn-danger");
                        traceBtn.classList.remove("btn-success");
                        traceBtn.innerHTML = `<img src="stop-button.png" alt="Stop Icon" style="${iconStyle}"> Stop`;
                    } else {
                        traceBtn.classList.add("btn-success");
                        traceBtn.classList.remove("btn-danger");
                        traceBtn.innerHTML = `<img src="play-button.png" alt="Play Icon" style="${iconStyle}"> Start`;
                    }
                }

                updateTraceBtn(tracing);

                // Add click event listener to toggle tracing
                traceBtn.addEventListener("click", async () => {
                    if (tracing) {
                        await fetch("/api/trace/end", { method: "POST" });
                        tracing = false;
                    } else {
                        await fetch("/api/trace/start", { method: "POST" });
                        tracing = true;
                    }
                    updateTraceBtn(tracing);
                });
            });
    }
}

const app = new App();
app.start()