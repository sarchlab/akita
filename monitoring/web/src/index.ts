
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

class App {
    uiManager: UIManager
    engineController: EngineController
    progressBarManager: ProgressBarManager

    constructor() {
        this.uiManager = new UIManager();
        this.engineController = new EngineController();
        this.progressBarManager = new ProgressBarManager(this.uiManager);
    }

    start() {
        this.uiManager.assignElements()
        this.uiManager.resize()

        this.engineController.bindDom()
        this.progressBarManager.binDom()

        window.addEventListener("resize", () => {
            this.uiManager.resize();
        });

        listComponents()
    }
}

const app = new App();
app.start()