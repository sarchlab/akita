
import './reset.css'
import './styles.scss'
import './style.css'

import '@fortawesome/fontawesome-free/js/fontawesome'
import '@fortawesome/fontawesome-free/js/solid'
import '@fortawesome/fontawesome-free/js/regular'

import './component_tree'
import { UIManager } from './ui_manager'
import { listComponents } from './component_tree'
import { EngineController } from './engine_control'
import { ProgressBarManager } from './progress_bar'
import { HangAnalyzer } from './hang_analyzer'
import { Monitor } from './monitor'
import { ResourceMonitor } from './resource'
import { setupTraceButton } from './tracing'

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

        const traceBtn = document.getElementById("trace-toggle-btn") as HTMLButtonElement | null;
        
        // Preload images to prevent flickering
        const preloadImages = () => {
            const rotateImg = new Image();
            rotateImg.src = "rotate_icon.png";
            const stopImg = new Image();
            stopImg.src = "stop-button.png";
        };
        preloadImages();

        if (traceBtn) {
            setupTraceButton(traceBtn);
        } else {
            console.warn("Trace toggle button not found.");
        }
    }
}

const app = new App();
app.start()
