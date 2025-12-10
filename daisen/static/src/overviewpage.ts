import Overview from "./overview";

class OverviewPage {
  _container: HTMLElement;
  _overview: Overview;

  constructor() {
    this._container = null;
    this._overview = new Overview();
  }

  layout() {
    const [_, containerHeight] = this._containerDim();
    this._container = document.getElementById("inner-container");
    this._container.style.height = (containerHeight - 60).toString() + "px";

    const overviewCanvas = this._getOverviewCanvas();
    const toolBar = this._getToolbar();
    toolBar.innerHTML = "";

    this._overview.setCanvas(overviewCanvas, toolBar);
  }

  _containerDim(): [number, number] {
    const container = document.getElementById("container");
    const topNav = document.getElementById("top-nav");
    return [container.offsetWidth, window.innerHeight - topNav.offsetHeight];
  }

  _getOverviewCanvas(): HTMLDivElement {
    const list = this._container.getElementsByClassName("overview-canvas");
    let overviewCanvas: HTMLDivElement;

    if (list.length > 0) {
      overviewCanvas = <HTMLDivElement>list[0];
    } else {
      overviewCanvas = document.createElement("div");
      overviewCanvas.classList.add("overview-canvas");
      overviewCanvas.classList.add("container-fluid");
      this._container.append(overviewCanvas);
    }

    const [containerWidth, containerHeight] = this._containerDim();
    overviewCanvas.style.width = containerWidth.toString() + "px";
    overviewCanvas.style.height = containerHeight.toString() + "px";

    return overviewCanvas;
  }

  _getToolbar(): HTMLFormElement {
    const nav = document.getElementById("top-nav");
    const forms = nav.getElementsByTagName("form");
    let toolBar = null;
    if (forms.length == 0) {
      toolBar = this._createToolbar(nav);
    } else {
      toolBar = forms[0];
    }
    return toolBar;
  }

  _createToolbar(nav: HTMLElement): HTMLFormElement {
    const toolBar = document.createElement("form");
    toolBar.id = "tool-bar";
    toolBar.classList.add("form-inline");
    toolBar.classList.add("mb-0");
    toolBar.onsubmit = e => {
      e.preventDefault();
    };
    nav.appendChild(toolBar);
    return toolBar;
  }

  render() {
    this._overview.render();
  }
}

export default OverviewPage;