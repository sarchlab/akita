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

        // Helper to update button UI
        function updateTraceBtn(tracing: boolean) {
            if (tracing) {
                traceBtn.classList.remove("btn-success");
                traceBtn.classList.add("btn-danger");
                traceBtn.textContent = "Stop";
            } else {
                traceBtn.classList.remove("btn-danger");
                traceBtn.classList.add("btn-success");
                traceBtn.textContent = "Start";
            }
        }

        // Fetch the initial tracing status from the backend
        fetch("/api/trace/is_tracing")
            .then((response) => response.json())
            .then((data) => {
                // Initialize button format based on backend response
                updateTraceBtn(data.isTracing);

                // Add click event listener to toggle tracing
                traceBtn.addEventListener("click", async () => {
                    if (data.isTracing) {
                        await fetch("/api/trace/end", { method: "POST" });
                        data.isTracing = false;
                    } else {
                        await fetch("/api/trace/start", { method: "POST" });
                        data.isTracing = true;
                    }
                    updateTraceBtn(data.isTracing);
                });
            })
            .catch((error) => {
                console.error("Failed to fetch tracing status:", error);
            });
    }
}

const app = new App();
app.start();