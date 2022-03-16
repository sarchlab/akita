
import './reset.css'
import 'bootstrap'
import 'bootstrap/dist/css/bootstrap.min.css'
import './style.css'

import '@fortawesome/fontawesome-free/js/fontawesome'
import '@fortawesome/fontawesome-free/js/solid'
// import '@fortawesome/fontawesome-free/js/regular'
// import '@fortawesome/fontawesome-free/js/brands'

import './component_tree'
import { UIManager } from './ui_manager'
import { listComponents } from './component_tree'
import { EngineController } from './engine_control'
import { ProgressBarManager } from './progress_bar'
import { HangAnalyzer } from './hang_analyzer'

class App {
    uiManager: UIManager
    engineController: EngineController
    progressBarManager: ProgressBarManager
    hangAnalyzer: HangAnalyzer

    constructor() {
        this.uiManager = new UIManager();
        this.engineController = new EngineController();
        this.progressBarManager = new ProgressBarManager(this.uiManager);
        this.hangAnalyzer = new HangAnalyzer();
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

        listComponents()
    }
}

const app = new App();
app.start()