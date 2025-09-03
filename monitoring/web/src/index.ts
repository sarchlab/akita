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
        
        // Preload images to prevent flickering
        const preloadImages = () => {
            const rotateImg = new Image();
            rotateImg.src = "rotate_icon.png";
            const stopImg = new Image();
            stopImg.src = "stop-button.png";
        };
        preloadImages();
        
        // Test function to verify button works (can be called from browser console)
        (window as any).testTraceButton = () => {
            console.log("Testing trace button...");
            const btn = document.getElementById("trace-toggle-btn") as HTMLButtonElement;
            if (btn) {
                btn.click();
                console.log("Button clicked! Check the UI changes.");
            } else {
                console.log("Button not found!");
            }
        };

        // Fetch the initial tracing status from the backend
        fetch("/api/trace/is_tracing")
            .then((response) => {
                if (!response.ok) {
                    throw new Error(`HTTP error! status: ${response.status}`);
                }
                return response.json();
            })
            .then((data) => {
                let tracing = data.isTracing; // Initialize tracing state

                function updateTraceBtn(tracing: boolean) {

                    const img = traceBtn.querySelector('img') as HTMLImageElement;
                    const textNode = traceBtn.childNodes[traceBtn.childNodes.length - 1];
                    
                    // Add loading class to prevent flickering
                    if (img) img.classList.add('loading');
                    
                    if (tracing) {
                        // If tracing: btn-danger, rotating png, "Stop" text
                        traceBtn.classList.add("btn-danger");
                        traceBtn.classList.remove("btn-success");
                        img.src = "rotate_icon.png";
                        img.alt = "Rotate Icon";
                        img.className = "rotating-icon";
                        textNode.textContent = " Stop";
                    } else {
                        // If not tracing: btn-success, static png, "Start" text
                        traceBtn.classList.add("btn-success");
                        traceBtn.classList.remove("btn-danger");
                        img.src = "rotate_icon.png";
                        img.alt = "Stop Icon";
                        img.className = "";
                        textNode.textContent = " Start";
                    }
                    
                    // Remove loading class after image loads
                    if (img) {
                        img.onload = () => img.classList.remove('loading');
                        img.onerror = () => img.classList.remove('loading');
                    }
                }

                updateTraceBtn(tracing);

                // Add click event listener to toggle tracing
                traceBtn.addEventListener("click", async () => {
                    try {
                        if (tracing) {
                            await fetch("/api/trace/end", { method: "POST" });
                            tracing = false;
                        } else {
                            await fetch("/api/trace/start", { method: "POST" });
                            tracing = true;
                        }
                        updateTraceBtn(tracing);
                    } catch (error) {
                        console.error("Error toggling trace:", error);
                    }
                });
            })
            .catch((error) => {
                console.error("Error fetching trace status:", error);
                // Set default state if API fails - start with not tracing
                let tracing = false;
                
                function updateTraceBtn(tracing: boolean) {

                    const img = traceBtn.querySelector('img') as HTMLImageElement;
                    const textNode = traceBtn.childNodes[traceBtn.childNodes.length - 1];
                    
                    // Add loading class to prevent flickering
                    if (img) img.classList.add('loading');
                    
                    if (tracing) {
                        // If tracing: btn-danger, rotating png, "Stop" text
                        traceBtn.classList.add("btn-danger");
                        traceBtn.classList.remove("btn-success");
                        img.src = "rotate_icon.png";
                        img.alt = "Rotate Icon";
                        img.className = "rotating-icon";
                        textNode.textContent = " Stop";
                    } else {
                        // If not tracing: btn-success, static png, "Start" text
                        traceBtn.classList.add("btn-success");
                        traceBtn.classList.remove("btn-danger");
                        img.src = "rotate_icon.png";
                        img.alt = "Stop Icon";
                        img.className = "";
                        textNode.textContent = " Start";
                    }
                    
                    // Remove loading class after image loads
                    if (img) {
                        img.onload = () => img.classList.remove('loading');
                        img.onerror = () => img.classList.remove('loading');
                    }
                }
                
                updateTraceBtn(tracing);
                
                // Add click event listener for offline mode - just toggle UI state
                traceBtn.addEventListener("click", () => {
                    tracing = !tracing;
                    updateTraceBtn(tracing);
                    console.log("Offline mode: Toggled tracing state to", tracing);
                });
            });
    }
}

const app = new App();
app.start()